package cli

import (
	"context"
	"net"
	"os"
	"path"
	"strings"

	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/github"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
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

func SplitTenantHost(fabric string) (pkg.TenantID, string) {
	parts := strings.SplitN(fabric, "@", 2)
	if len(parts) == 2 {
		return pkg.TenantID(parts[0]), parts[1]
	}
	return pkg.DEFAULT_TENANT, fabric
}

func CheckLogin(ctx context.Context, client client.Client) error {
	// TODO: create a proper rpc for this; or we can use a refresh token and use that to check
	fabric := client.GetFabric()
	tenant, _ := SplitTenantHost(fabric)
	_, err := client.RefreshToken(ctx, &v1.RefreshTokenRequest{
		Tenant:             tenant.String(),
		CurrentAccessToken: GetExistingToken(fabric),
	})
	return err
}

func Login(ctx context.Context, client client.Client, clientId, fabric string) (string, error) {
	Debug(" - Logging in to", fabric)

	code, err := github.StartAuthCodeFlow(ctx, clientId)
	if err != nil {
		return "", err
	}

	tenant, _ := SplitTenantHost(fabric)
	return generateToken(ctx, client, code, tenant, 0) // no scopes = unrestricted
}

func SaveAccessToken(fabric, at string) error {
	tokenFile := getTokenFile(fabric)
	os.MkdirAll(tokenDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(at), 0600); err != nil {
		return err
	}
	Debug(" - Access token saved to", tokenFile)
	return nil
}

func LoginAndSaveAccessToken(ctx context.Context, client client.Client, clientId, fabric string) error {
	at, err := Login(ctx, client, clientId, fabric)
	if err != nil {
		return err
	}

	tenant, host := SplitTenantHost(fabric)
	Info(" * Successfully logged in to", host, "("+tenant.String()+" tenant)")

	if err := SaveAccessToken(fabric, at); err != nil {
		Warn(" ! Failed to save access token:", err)
	}
	return nil
}
