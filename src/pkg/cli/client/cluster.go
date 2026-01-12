package client

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const DefaultCluster = "fabric-prod1.defang.dev"

var DefangFabric = pkg.Getenv("DEFANG_FABRIC", DefaultCluster)

func NormalizeHost(cluster string) string {
	if cluster == "" {
		cluster = DefangFabric
	}
	if _, _, err := net.SplitHostPort(cluster); err != nil {
		cluster = cluster + ":443" // default to https
	}
	return cluster
}

func tokenStorageName(cluster string) string {
	// Token files are keyed by normalized host (no tenant prefix, no port) to avoid duplication.
	if at := strings.LastIndex(cluster, "@"); at >= 0 && at < len(cluster)-1 {
		cluster = cluster[at+1:] // drop legacy tenant prefix
	}
	host := NormalizeHost(cluster)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil && parsedHost != "" {
		host = parsedHost
	}
	return host
}

func GetTokenFile(cluster string) string {
	return filepath.Join(StateDir, tokenStorageName(cluster))
}

func GetExistingToken(cluster string) string {
	var accessToken = os.Getenv("DEFANG_ACCESS_TOKEN")

	if accessToken != "" {
		term.Debug("Using access token from env DEFANG_ACCESS_TOKEN")
	} else {
		tokenFile := GetTokenFile(cluster)

		term.Debug("Reading access token from file", tokenFile)
		all, _ := os.ReadFile(tokenFile)
		accessToken = string(all) // might be empty

		if jwtPath, err := GetWebIdentityTokenFile(cluster); err == nil {
			if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") == "" {
				term.Debugf("using web identity token from %s", jwtPath)
				// Set AWS env vars for this CLI invocation
				os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", jwtPath)
				os.Setenv("AWS_ROLE_SESSION_NAME", "defang-cli") // TODO: from WhoAmI
			} else {
				term.Debugf("AWS_WEB_IDENTITY_TOKEN_FILE is already set; not using token file")
			}
		}
	}

	return accessToken
}

func GetWebIdentityTokenFile(cluster string) (string, error) {
	jwtPath := GetTokenFile(cluster) + ".jwt" // TODO: store in TMPDIR instead?
	_, err := os.Stat(jwtPath)
	return jwtPath, err
}

func SaveAccessToken(cluster, token string) error {
	tokenFile := GetTokenFile(cluster)
	term.Debug("Saving access token to", tokenFile)
	os.MkdirAll(StateDir, 0700)
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save access token: %w", err)
	}
	return nil
}
