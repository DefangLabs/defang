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
	Kid string   `json:"kid,omitempty"`
	Alg string   `json:"alg,omitempty"`
	Use string   `json:"use,omitempty"`
	N   string   `json:"n,omitempty"`   // RSA modulus, base64url-encoded
	E   string   `json:"e,omitempty"`   // RSA exponent, base64url-encoded
	X5c [][]byte `json:"x5c,omitempty"` // DER-encoded cert(s)
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
		if key.X5t != "" {
			thumbprint, err := base64.RawURLEncoding.DecodeString(key.X5t)
			if err != nil {
				return nil, fmt.Errorf("invalid base64url encoding in x5t claim: %w", err)
			}
			thumbprints = append(thumbprints, hex.EncodeToString(thumbprint))
		} else if len(key.X5c) > 0 {
			// Compute SHA-1 thumbprint of DER-encoded cert
			// thumbprint := sha1.Sum(key.X5c[0]) not important; avoid importing sha1
			// thumbprints = append(thumbprints, hex.EncodeToString(thumbprint[:]))
		}
	}
	return thumbprints, nil
}
