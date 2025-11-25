package cfn

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/http"
)

type Jwk struct {
	Kty string   `json:"kty"`
	Alg string   `json:"alg,omitempty"`
	Use string   `json:"use,omitempty"`
	X5c []string `json:"x5c,omitempty"`
	X5t string   `json:"x5t,omitempty"` // base64url-encoded
}

type JwkSet struct {
	Keys []Jwk `json:"keys"`
}

type OpenIdConfiguration struct {
	JwksUri string `json:"jwks_uri"`
}

var httpClient = http.DefaultClient

func FetchThumbprints(iss string) ([]string, error) {
	resp, err := httpClient.Get("https://" + iss + "/.well-known/openid-configuration")
	if err != nil {
		return nil, fmt.Errorf("failed to create openid-configuration HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch openid-configuration: status %s", resp.Status)
	}

	var oidConfig OpenIdConfiguration
	if err := json.NewDecoder(resp.Body).Decode(&oidConfig); err != nil {
		return nil, fmt.Errorf("could not decode response from openid-configuration: %w", err)
	}

	resp2, err := httpClient.Get(oidConfig.JwksUri)
	if err != nil {
		return nil, fmt.Errorf("failed to create jwks_uri HTTP request: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch jwks_uri: status %s", resp2.Status)
	}

	var jwks JwkSet
	if err := json.NewDecoder(resp2.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("could not decode response from jwks_uri: %w", err)
	}

	var thumbprints []string
	for _, key := range jwks.Keys {
		if len(key.X5c) > 0 {
			decoded, err := base64.RawURLEncoding.DecodeString(key.X5t)
			if err != nil {
				return nil, fmt.Errorf("invalid base64url encoding in x5t claim: %w", err)
			}
			thumbprints = append(thumbprints, hex.EncodeToString(decoded))
		}
	}
	return thumbprints, nil
}
