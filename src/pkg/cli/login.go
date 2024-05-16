package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

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

		term.Debug(" - Reading access token from file", tokenFile)
		all, _ := os.ReadFile(tokenFile)
		accessToken = string(all)
	} else {
		term.Debug(" - Using access token from env DEFANG_ACCESS_TOKEN")
	}

	return accessToken
}

func loginWithGitHub(ctx context.Context, client client.Client, gitHubClientId, fabric string) (string, error) {
	term.Debug(" - Logging in to", fabric)

	code, err := github.StartAuthCodeFlow(ctx, gitHubClientId)
	if err != nil {
		return "", err
	}

	tenant, _ := SplitTenantHost(fabric)
	return exchangeCodeForToken(ctx, client, code, tenant, 0) // no scopes = unrestricted
}

func saveAccessToken(fabric, at string) error {
	tokenFile := getTokenFile(fabric)
	term.Debug(" - Saving access token to", tokenFile)
	os.MkdirAll(client.StateDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(at), 0600); err != nil {
		return err
	}
	return nil
}

func InteractiveLogin(ctx context.Context, client client.Client, gitHubClientId, fabric string) error {
	at, err := loginWithGitHub(ctx, client, gitHubClientId, fabric)
	if err != nil {
		return err
	}

	tenant, host := SplitTenantHost(fabric)
	term.Info(" * Successfully logged in to", host, "("+tenant.String()+" tenant)")

	if err := saveAccessToken(fabric, at); err != nil {
		term.Warn(" ! Failed to save access token:", err)
	}
	return nil
}

func NonInteractiveLogin(ctx context.Context, client client.Client, fabric string) error {
	term.Debug(" - Non-interactive login using GitHub Actions id-token")
	idToken, err := github.GetIdToken(ctx)
	if err != nil {
		return fmt.Errorf("non-interactive login failed: %w", err)
	}
	term.Debug(" - Got GitHub Actions id-token")
	resp, err := client.Token(ctx, &defangv1.TokenRequest{
		Assertion: idToken,
		Scope:     []string{"admin", "read"},
	})
	if err != nil {
		return err
	}
	return saveAccessToken(fabric, resp.AccessToken)
}
