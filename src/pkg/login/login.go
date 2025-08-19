package login

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const DefaultCluster = "fabric-prod1.defang.dev"

var DefangFabric = pkg.Getenv("DEFANG_FABRIC", DefaultCluster)

func SplitTenantHost(cluster string) (types.TenantName, string) {
	tenant := types.DEFAULT_TENANT
	parts := strings.SplitN(cluster, "@", 2)
	if len(parts) == 2 {
		tenant, cluster = types.TenantName(parts[0]), parts[1]
	}
	if cluster == "" {
		cluster = DefangFabric
	}
	if _, _, err := net.SplitHostPort(cluster); err != nil {
		cluster = cluster + ":443" // default to https
	}
	return tenant, cluster
}

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

type LoginFlow = auth.LoginFlow

type AuthService interface {
	login(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow) (string, error)
	serveAuthServer(ctx context.Context, fabric string, authPort int) error
}

type OpenAuthService struct{}

func (g OpenAuthService) login(ctx context.Context, client client.FabricClient, fabric string, flow LoginFlow) (string, error) {
	term.Debug("Logging in to", fabric)

	code, err := auth.StartAuthCodeFlow(ctx, flow)
	if err != nil {
		return "", err
	}

	tenant, _ := SplitTenantHost(fabric)
	return auth.ExchangeCodeForToken(ctx, code, tenant, 0) // no scopes = unrestricted
}

func (g OpenAuthService) serveAuthServer(ctx context.Context, fabric string, authPort int) error {
	term.Debug("Logging in to", fabric)

	tenant, _ := SplitTenantHost(fabric)

	err := auth.ServeAuthCodeFlowServer(ctx, authPort, tenant, func(token string) {
		saveAccessToken(fabric, token)
	})
	if err != nil {
		term.Error("failed to start auth server", "error", err)
	}
	return nil
}

var authService AuthService = OpenAuthService{}

func saveAccessToken(fabric, token string) error {
	tokenFile := getTokenFile(fabric)
	term.Debug("Saving access token to", tokenFile)
	os.MkdirAll(client.StateDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save access token: %w", err)
	}
	return nil
}

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

	tenant, host := SplitTenantHost(fabric)
	term.Info("Successfully logged in to", host, "("+tenant.String()+" tenant)")

	if err := saveAccessToken(fabric, token); err != nil {
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
	return saveAccessToken(fabric, resp.AccessToken)
}
