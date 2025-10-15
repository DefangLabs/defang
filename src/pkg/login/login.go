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
	login(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow) (string, error)
	serveAuthServer(ctx context.Context, fabric string, authPort int) error
}

type OpenAuthService struct{}

func (g OpenAuthService) login(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow) (string, error) {
	term.Debug("Logging in to", fabric)

	code, err := auth.StartAuthCodeFlow(ctx, flow, func(token string) {
		cluster.SaveAccessToken(fabric, token)
	})
	if err != nil {
		return "", err
	}

	return auth.ExchangeCodeForToken(ctx, code) // no scopes = unrestricted
}

func (g OpenAuthService) serveAuthServer(ctx context.Context, fabric string, authPort int) error {
	term.Debug("Logging in to", fabric)

	tenant, _ := cluster.SplitTenantHost(fabric)

	err := auth.ServeAuthCodeFlowServer(ctx, authPort, tenant, func(token string) {
		cluster.SaveAccessToken(fabric, token)
	})
	if err != nil {
		term.Error("failed to start auth server", "error", err)
	}
	return nil
}

var authService AuthService = OpenAuthService{}

func InteractiveLogin(ctx context.Context, client client.FabricClient, fabric string) error {
	return interactiveLogin(ctx, client, fabric, auth.CliFlow)
}

func InteractiveLoginMCP(ctx context.Context, client client.FabricClient, fabric string) error {
	return interactiveLogin(ctx, client, fabric, auth.McpFlow)
}

func InteractiveLoginInsideDocker(ctx context.Context, fabric string, authPort int) error {
	return authService.serveAuthServer(ctx, fabric, authPort)
}

func interactiveLogin(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow) error {
	token, err := authService.login(ctx, client, fabric, flow)
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
	return cluster.SaveAccessToken(fabric, resp.AccessToken)
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
