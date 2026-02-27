package login

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type LoginFlow = auth.LoginFlow

type AuthService interface {
	login(ctx context.Context, fabricAddr string, flow LoginFlow, mcpClient string) (string, error)
}

type OpenAuthService struct{}

func (OpenAuthService) login(ctx context.Context, fabricAddr string, flow LoginFlow, mcpClient string) (string, error) {
	term.Debug("Logging in to", fabricAddr)

	code, err := auth.StartAuthCodeFlow(ctx, flow, func(token string) {
		client.SaveAccessToken(fabricAddr, token)
	}, mcpClient)
	if err != nil {
		return "", err
	}

	return auth.ExchangeCodeForToken(ctx, code) // no scopes = unrestricted
}

var authService AuthService = OpenAuthService{}

func InteractiveLogin(ctx context.Context, fabricAddr string) error {
	return interactiveLogin(ctx, fabricAddr, auth.CliFlow, "CLI-Flow")
}

func InteractiveLoginMCP(ctx context.Context, fabricAddr string, mcpClient string) error {
	return interactiveLogin(ctx, fabricAddr, auth.McpFlow, mcpClient)
}

func interactiveLogin(ctx context.Context, fabricAddr string, flow LoginFlow, mcpClient string) error {
	token, err := authService.login(ctx, fabricAddr, flow, mcpClient)
	if err != nil {
		return err
	}

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	if err := client.SaveAccessToken(fabricAddr, token); err != nil {
		term.Warn(err)
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			term.Printf("\nTo fix file permissions, run:\n\n  sudo chown -R $(whoami) %q\n", pathError.Path)
		}
		// We continue even if we can't save the token; we just won't have it saved for next time
	}
	// The new login page shows the ToS so a successful login implies the user agreed
	return nil
}

func NonInteractiveGitHubLogin(ctx context.Context, fabric client.FabricClient, fabricAddr string) error {
	term.Debug("Non-interactive login using GitHub Actions id-token")
	idToken, err := github.GetIdToken(ctx, "") // default audience (ie. https://github.com/ORG)
	if err != nil {
		return fmt.Errorf("non-interactive login failed: %w", err)
	}
	term.Debug("Got GitHub Actions id-token")

	// Create a Fabric token using the GitHub token as an assertion
	resp, err := fabric.Token(ctx, &defangv1.TokenRequest{
		Assertion: idToken,
		Scope:     []string{"admin", "read", "delete", "tail"},
	})
	if err != nil {
		return err
	}

	err = client.SaveAccessToken(fabricAddr, resp.AccessToken) // creates the state folder too

	// If AWS_ROLE_ARN is set, we're doing "Assume Role with Web Identity"
	if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") == "" {
		// AWS_ROLE_ARN is set, but AWS_WEB_IDENTITY_TOKEN_FILE is empty: write the token to a new file
		jwtPath, _ := client.GetWebIdentityTokenFile(fabricAddr)
		term.Debugf("writing web identity token to %s", jwtPath)
		if err := os.WriteFile(jwtPath, []byte(idToken), 0600); err != nil {
			return fmt.Errorf("failed to save web identity token: %w", err)
		}
		// Set AWS env vars for this CLI invocation
		os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", jwtPath)
		os.Setenv("AWS_ROLE_SESSION_NAME", "testyml") // TODO: from WhoAmI
	} else {
		term.Debugf("AWS_WEB_IDENTITY_TOKEN_FILE is already set; not writing token to a new file")
	}

	return err
}

// InteractiveRequireLoginAndToS ensures the user is logged in and has agreed to the terms of service.
// If necessary, it will reconnect to the server as the right tenant, returning the updated Fabric client.
func InteractiveRequireLoginAndToS(ctx context.Context, fabric client.FabricClient, fabricAddr string) (client.FabricClient, error) {
	var err error
	if err = fabric.CheckLoginAndToS(ctx); err != nil {
		// Login interactively now; only do this for authorization-related errors
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			term.Debug("Server error:", err)
			term.Warn("Please log in to continue.")
			term.ResetWarnings() // clear any previous warnings so we don't show them again

			defer func() { track.Cmd(nil, "Login", P("reason", err)) }()
			if err = InteractiveLogin(ctx, fabricAddr); err != nil {
				return fabric, err
			}

			// Reconnect with the new token
			if newFabric, err := cli.ConnectWithTenant(ctx, fabricAddr, fabric.GetRequestedTenant()); err != nil {
				return fabric, err
			} else {
				fabric = newFabric
				track.Tracker = fabric // update global tracker
			}

			if err = fabric.CheckLoginAndToS(ctx); err == nil { // recheck (new token = new user)
				return fabric, nil // success
			}
		}

		// Check if the user has agreed to the terms of service and show a prompt if needed
		if connect.CodeOf(err) == connect.CodeFailedPrecondition {
			term.Warn(client.PrettyError(err))

			defer func() { track.Cmd(nil, "Terms", P("reason", err)) }()
			if err = InteractiveAgreeToS(ctx, fabric); err != nil {
				return fabric, err // fatal
			}
		}
	}
	return fabric, err
}
