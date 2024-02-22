package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"path"

	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/github"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

var (
	tokenDir = path.Join(pkg.Getenv("XDG_STATE_HOME", path.Join(os.Getenv("HOME"), ".local/state")), "defang")
)

func getTokenFile(fabric string) string {
	if host, _, _ := net.SplitHostPort(fabric); host != "" {
		fabric = host
	}
	return path.Join(tokenDir, fabric)
}

func GetExistingToken(fabric string) string {
	var accessToken = os.Getenv("DEFANG_ACCESS_TOKEN")

	if accessToken == "" {
		tokenFile := getTokenFile(fabric)

		Debug(" - Reading access token from file", tokenFile)
		all, _ := os.ReadFile(tokenFile)
		accessToken = string(all)
	} else {
		Debug(" - Using access token from env DEFANG_ACCESS_TOKEN")
	}

	return accessToken
}

func LoginWithGitHub(ctx context.Context, client client.Client, gitHubClientId, fabric string) (string, error) {
	Debug(" - Logging in to", fabric)

	code, err := github.StartAuthCodeFlow(ctx, gitHubClientId)
	if err != nil {
		return "", err
	}

	tenant, _ := SplitTenantHost(fabric)
	return exchangeCodeForToken(ctx, client, code, tenant, 0) // no scopes = unrestricted
}

func saveAccessToken(fabric, at string) error {
	tokenFile := getTokenFile(fabric)
	Debug(" - Saving access token to", tokenFile)
	os.MkdirAll(tokenDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(at), 0600); err != nil {
		return err
	}
	return nil
}

func InteractiveLogin(ctx context.Context, client client.Client, gitHubClientId, fabric string) error {
	at, err := LoginWithGitHub(ctx, client, gitHubClientId, fabric)
	if err != nil {
		return err
	}

	tenant, host := SplitTenantHost(fabric)
	Info(" * Successfully logged in to", host, "("+tenant.String()+" tenant)")

	if err := saveAccessToken(fabric, at); err != nil {
		Warn(" ! Failed to save access token:", err)
	}
	return nil
}

func NonInteractiveLogin(ctx context.Context, client client.Client, fabric string) error {
	Debug(" - Non-interactive login using GitHub Actions id-token")
	idToken, err := github.GetIdToken(ctx)
	if err != nil {
		return fmt.Errorf("non-interactive login failed: %w", err)
	}
	Debug(" - Got GitHub Actions id-token")
	resp, err := client.Token(ctx, &defangv1.TokenRequest{
		Assertion: idToken,
		Scope:     []string{"admin", "read"},
	})
	if err != nil {
		return err
	}
	return saveAccessToken(fabric, resp.AccessToken)
}
