// Package identity implements the client side of the Defang agent-identity
// key registry: local keypair management, proof-of-possession signing, and
// the HTTP client for the per-tenant OIDC key registry (/keys endpoints).
package identity

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	_ "crypto/sha256" // registered for JSONWebKey.Thumbprint
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
)

// RS256 because AWS OIDC federation does not accept Ed25519, making RSA the
// portable choice across cloud identity providers.
const rsaKeyBits = 2048

const privateKeyFile = "private.pem"

// Key is a locally-held agent keypair. Only the public half is ever sent to
// the registry.
type Key struct {
	private *rsa.PrivateKey
}

// LoadOrGenerateKey returns the keypair stored in dir, generating and
// persisting a new one if none exists yet.
func LoadOrGenerateKey(dir string) (*Key, error) {
	path := filepath.Join(dir, privateKeyFile)
	if pemBytes, err := os.ReadFile(path); err == nil {
		return parsePrivateKey(pemBytes, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	private, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0600); err != nil {
		return nil, err
	}
	return &Key{private: private}, nil
}

func parsePrivateKey(pemBytes []byte, path string) (*Key, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key %s: %w", path, err)
	}
	private, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: expected an RSA private key, got %T", path, parsed)
	}
	return &Key{private: private}, nil
}

// PublicJWK returns the minimal public JWK (kty/n/e). The registry derives
// kid, alg, and use server-side; sending them would only invite mismatches.
func (k *Key) PublicJWK() jose.JSONWebKey {
	return jose.JSONWebKey{Key: k.private.Public()}
}

// Thumbprint returns the RFC 7638 JWK thumbprint (base64url), which the
// registry also uses as the JWKS kid.
func (k *Key) Thumbprint() (string, error) {
	jwk := k.PublicJWK()
	thumb, err := jwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(thumb), nil
}

// PopJWT signs the proof-of-possession token required by the registry: a
// fresh JWT binding the registration request to the private key, so a stolen
// public JWK can't be registered by someone who doesn't hold the private key.
func (k *Key) PopJWT(now time.Time) (string, error) {
	thumb, err := k.Thumbprint()
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims{
		"iat":       now.Unix(),
		"jwk_thumb": thumb,
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(k.private)
}
