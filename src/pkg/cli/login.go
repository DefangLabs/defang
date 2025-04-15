package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func getTokenFile(fabric string) string {
	if host, _, _ := net.SplitHostPort(fabric); host != "" {
		fabric = host
	}
	return filepath.Join(client.StateDir, fabric)
}

func GetExistingToken(fabric string) string {
	var accessToken = os.Getenv("DEFANG_ACCESS_TOKEN")

	if accessToken == "" {
		tokenFile := getTokenFile(fabric)

		term.Debug("Reading access token from file", tokenFile)
		all, _ := os.ReadFile(tokenFile)
		accessToken = string(all)
	} else {
		term.Debug("Using access token from env DEFANG_ACCESS_TOKEN")
	}

	return accessToken
}

type AuthService interface {
	login(ctx context.Context, client client.FabricClient, gitHubClientId, fabric string, prompt bool) (string, error)
}

type OpenAuthService struct{}

func (g OpenAuthService) login(
	ctx context.Context, client client.FabricClient, gitHubClientId, fabric string, prompt bool,
) (string, error) {
	term.Debug("Logging in to", fabric)

	code, err := auth.StartAuthCodeFlow(ctx, gitHubClientId, prompt)
	if err != nil {
		return "", err
	}

	tenant, _ := SplitTenantHost(fabric)
	return auth.ExchangeCodeForToken(ctx, code, tenant, 0) // no scopes = unrestricted
}

var githubAuthService AuthService = OpenAuthService{}

func saveAccessToken(fabric, at string) error {
	tokenFile := getTokenFile(fabric)
	term.Debug("Saving access token to", tokenFile)
	os.MkdirAll(client.StateDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(at), 0600); err != nil {
		return err
	}
	return nil
}

func InteractiveLogin(ctx context.Context, client client.FabricClient, gitHubClientId, fabric string, prompt bool) error {
	at, err := githubAuthService.login(ctx, client, gitHubClientId, fabric, prompt)
	if err != nil {
		return err
	}

	tenant, host := SplitTenantHost(fabric)
	term.Info("Successfully logged in to", host, "("+tenant.String()+" tenant)")

	if err := saveAccessToken(fabric, at); err != nil {
		term.Warnf("Failed to save access token, try re-authenticating: %v", err)
	}
	return nil
}

func NonInteractiveLogin(ctx context.Context, client client.FabricClient, fabric string) error {
	term.Debug("Non-interactive login using GitHub Actions id-token")
	idToken, err := github.GetIdToken(ctx)
	if err != nil {
		return fmt.Errorf("non-interactive login failed: %w", err)
	}
	term.Debug("Got GitHub Actions id-token")
	resp, err := client.Token(ctx, &defangv1.TokenRequest{
		Assertion: idToken,
		Scope:     []string{"admin", "read"}, // no "tail" scope
	})
	if err != nil {
		return err
	}
	return saveAccessToken(fabric, resp.AccessToken)
}
