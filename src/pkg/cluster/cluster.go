package cluster

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const DefaultCluster = "fabric-prod1.defang.dev"

var DefaultAccessToken = ""

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

func GetTokenFile(fabric string) string {
	if host, _, _ := net.SplitHostPort(fabric); host != "" {
		fabric = host
	}
	return filepath.Join(client.StateDir, fabric)
}

func GetExistingToken(fabric string) string {
	if DefaultAccessToken != "" {
		return DefaultAccessToken
	}

	var accessToken = os.Getenv("DEFANG_ACCESS_TOKEN")

	if accessToken == "" {
		tokenFile := GetTokenFile(fabric)

		term.Debug("Reading access token from file", tokenFile)
		all, _ := os.ReadFile(tokenFile)
		accessToken = string(all)
	} else {
		term.Debug("Using access token from env DEFANG_ACCESS_TOKEN", accessToken)
	}

	return accessToken
}

func SaveAccessToken(fabric, token string) error {
	tokenFile := GetTokenFile(fabric)
	term.Debug("Saving access token to", tokenFile)
	os.MkdirAll(client.StateDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save access token: %w", err)
	}
	return nil
}
