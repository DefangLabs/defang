package login

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type LoginFlow = auth.LoginFlow

type AuthService interface {
	login(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow, mcpClient string) (string, error)
}

type OpenAuthService struct{}

func (OpenAuthService) login(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow, mcpClient string) (string, error) {
	term.Debug("Logging in to", fabric)

	code, err := auth.StartAuthCodeFlow(ctx, flow, func(token string) {
		cluster.SaveAccessToken(fabric, token)
	}, mcpClient)
	if err != nil {
		return "", err
	}

	return auth.ExchangeCodeForToken(ctx, code) // no scopes = unrestricted
}

var authService AuthService = OpenAuthService{}

func InteractiveLogin(ctx context.Context, client client.FabricClient, fabric string) error {
	return interactiveLogin(ctx, client, fabric, auth.CliFlow, "CLI-Flow")
}

func InteractiveLoginMCP(ctx context.Context, client client.FabricClient, fabric string, mcpClient string) error {
	return interactiveLogin(ctx, client, fabric, auth.McpFlow, mcpClient)
}

func interactiveLogin(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow, mcpClient string) error {
	token, err := authService.login(ctx, client, fabric, flow, mcpClient)
	if err != nil {
		return err
	}

	tenant, host := cluster.SplitTenantHost(fabric)
	term.Info("Successfully logged in to", host, "("+tenant.String()+" tenant)")

	if err := cluster.SaveAccessToken(fabric, token); err != nil {
		term.Warn(err)
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			term.Printf("\nTo fix file permissions, run:\n\n  sudo chown -R $(whoami) %q\n", pathError.Path)
		}
		// We continue even if we can't save the token; we just won't have it saved for next time
	}
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}
	// The new login page shows the ToS so a successful login implies the user agreed
	if err := NonInteractiveAgreeToS(ctx, client); err != nil {
		term.Debug("unable to agree to terms:", err) // not fatal
	}
	return nil
}

func NonInteractiveGitHubLogin(ctx context.Context, client client.FabricClient, fabric string) error {
	term.Debug("Non-interactive login using GitHub Actions id-token")
	idToken, err := github.GetIdToken(ctx)
	if err != nil {
		return fmt.Errorf("non-interactive login failed: %w", err)
	}
	term.Debug("Got GitHub Actions id-token")
	resp, err := client.Token(ctx, &defangv1.TokenRequest{
		Assertion: idToken,
		Scope:     []string{"admin", "read", "delete", "tail"},
	})
	if err != nil {
		return err
	}

	err = cluster.SaveAccessToken(fabric, resp.AccessToken) // creates the state folder too

	if roleArn := os.Getenv("AWS_ROLE_ARN"); roleArn != "" {
		if file := os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"); file == "" {
			// AWS_ROLE_ARN is set, but AWS_WEB_IDENTITY_TOKEN_FILE is empty: write the token to a file
			jwtPath := cluster.GetTokenFile(fabric) + ".jwt"
			term.Debugf("writing web identity token to %s for role %s", jwtPath, roleArn)
			if err := os.WriteFile(jwtPath, []byte(idToken), 0600); err != nil {
				return err
			}
			os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", jwtPath) // only for this invocation
			os.Setenv("AWS_ROLE_SESSION_NAME", "testyml")     // only for this invocation
		} else {
			term.Debugf("AWS_WEB_IDENTITY_TOKEN_FILE is already set to %q; not writing token to a new file", file)
		}
	}

	return err
}

func InteractiveRequireLoginAndToS(ctx context.Context, fabric *client.GrpcClient, addr string) error {
	var err error
	if err = fabric.CheckLoginAndToS(ctx); err != nil {
		// Login interactively now; only do this for authorization-related errors
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			term.Debug("Server error:", err)
			term.Warn("Please log in to continue.")
			term.ResetWarnings() // clear any previous warnings so we don't show them again

			defer func() { track.Cmd(nil, "Login", P("reason", err)) }()
			if err = InteractiveLogin(ctx, fabric, addr); err != nil {
				return err
			}

			// Reconnect with the new token
			if newFabric, err := cli.Connect(ctx, addr); err != nil {
				return err
			} else {
				*fabric = *newFabric
			}

			if err = fabric.CheckLoginAndToS(ctx); err == nil { // recheck (new token = new user)
				return nil // success
			}
		}

		// Check if the user has agreed to the terms of service and show a prompt if needed
		if connect.CodeOf(err) == connect.CodeFailedPrecondition {
			term.Warn(client.PrettyError(err))

			defer func() { track.Cmd(nil, "Terms", P("reason", err)) }()
			if err = InteractiveAgreeToS(ctx, fabric); err != nil {
				return err // fatal
			}
		}
	}

	return err
}
