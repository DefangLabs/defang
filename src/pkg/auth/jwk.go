package auth

import (
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
)

type Jwk struct {
	Kty    string   `json:"kty"`
	Kid    string   `json:"kid,omitempty"`
	Use    string   `json:"use,omitempty"` // "sig" or "enc"
	Alg    string   `json:"alg,omitempty"`
	KeyOps []string `json:"key_ops,omitempty"`
	D      string   `json:"d,omitempty"`   // private keys
	E      string   `json:"e,omitempty"`   // RSA keys
	N      string   `json:"n,omitempty"`   // RSA keys
	Crv    string   `json:"crv,omitempty"` // EC keys
	X      string   `json:"x,omitempty"`   // EC keys
	Y      string   `json:"y,omitempty"`   // EC keys
}

type Jwks struct {
	Keys []Jwk `json:"keys"`
}

func (j *Jwk) PublicKey() (crypto.PublicKey, error) {
	switch j.Kty {
	case "RSA":
		n, err := bigFromBase64(j.N)
		if err != nil {
			return nil, err
		}
		e, err := bigFromBase64(j.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %s", j.Kty)
	}
}

func bigToBase64(b *big.Int) string {
	return base64.RawURLEncoding.EncodeToString(b.Bytes())
}

func bigFromBase64(s string) (*big.Int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return big.NewInt(0).SetBytes(b), err
}

func GetJwks(url string) (*Jwks, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	jwks := &Jwks{}
	err = json.NewDecoder(resp.Body).Decode(jwks)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}
	return jwks, nil
}
