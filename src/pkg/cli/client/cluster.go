package client

import (
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
)

const DefaultFabricAddr = "fabric-prod1.defang.dev"

var DefangFabric = pkg.Getenv("DEFANG_FABRIC", DefaultFabricAddr)
var TokenStore tokenstore.TokenStore = &tokenstore.LocalDirTokenStore{Dir: StateDir}

func NormalizeHost(fabricAddr string) string {
	if fabricAddr == "" {
		fabricAddr = DefangFabric
	}
	if _, _, err := net.SplitHostPort(fabricAddr); err != nil {
		fabricAddr = fabricAddr + ":443" // default to https
	}
	return fabricAddr
}

func TokenStorageName(fabricAddr string) string {
	// Token files are keyed by normalized host (no tenant prefix, no port) to avoid duplication.
	if at := strings.LastIndex(fabricAddr, "@"); at >= 0 && at < len(fabricAddr)-1 {
		fabricAddr = fabricAddr[at+1:] // drop legacy tenant prefix
	}
	host := NormalizeHost(fabricAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil && parsedHost != "" {
		host = parsedHost
	}
	return host
}

func GetTokenFile(fabricAddr string) string {
	return filepath.Join(StateDir, TokenStorageName(fabricAddr))
}

func GetExistingToken(fabricAddr string) string {
	var accessToken = os.Getenv("DEFANG_ACCESS_TOKEN")

	if accessToken != "" {
		term.Debug("Using access token from env DEFANG_ACCESS_TOKEN")
	} else {
		var err error
		accessToken, err = TokenStore.Load(TokenStorageName(fabricAddr))
		if err != nil {
			term.Debugf("failed to load access token for %v: %v", fabricAddr, err)
		}

		// Check if we wrote an IDToken file during login, if AWS_WEB_IDENTITY_TOKEN_FILE is empty,
		if os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") == "" {
			if jwtPath, err := GetWebIdentityTokenFile(fabricAddr); err == nil {
				term.Debugf("using web identity token from %s", jwtPath)
				// Set AWS env vars for this CLI invocation
				os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", jwtPath)
				os.Setenv("AWS_ROLE_SESSION_NAME", "defang-cli") // TODO: from WhoAmI
			}
		}
	}

	return accessToken
}

func GetWebIdentityTokenFile(fabricAddr string) (string, error) {
	jwtPath := filepath.Join(StateDir, TokenStorageName(fabricAddr)) + ".jwt" // TODO: store in TMPDIR instead?
	_, err := os.Stat(jwtPath)
	return jwtPath, err
}

func SaveAccessToken(fabricAddr, token string) error {
	return TokenStore.Save(TokenStorageName(fabricAddr), token)
}
