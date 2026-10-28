// Package identity is the client side of id.defang.io: local keypair
// management, proof-of-possession signing, and the HTTP client for a tenant's
// OIDC issuer. Only public keys leave this machine.
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

// RS256 because it is the only algorithm Azure workload identity federation
// supports; AWS and GCP accept it too, and none of the three accept Ed25519.
const rsaKeyBits = 2048

const privateKeyFile = "private.pem"

// Key is a locally held keypair. Only the public half is ever sent to the
// issuer.
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

// PublicJWK returns the minimal public JWK (kty/n/e). The issuer derives
// kid, alg, and use server-side; sending them would only invite mismatches.
func (k *Key) PublicJWK() jose.JSONWebKey {
	return jose.JSONWebKey{Key: k.private.Public()}
}

// Thumbprint returns the RFC 7638 JWK thumbprint (base64url), which the
// issuer also uses as the JWKS kid.
func (k *Key) Thumbprint() (string, error) {
	jwk := k.PublicJWK()
	thumb, err := jwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(thumb), nil
}

// ProofType marks a proof of possession so it can never pass for an
// ordinary token from the tenant issuer.
const ProofType = "key-proof+jwt"

// ProofJWT signs the proof of possession the issuer requires: a short-lived
// JWT naming this key's thumbprint, signed by this key, with the tenant's
// /keys endpoint as its audience. Someone holding only the public JWK cannot
// make one, and a captured proof cannot enroll the key in another tenant or
// be exchanged for cloud credentials.
func (k *Key) ProofJWT(now time.Time, issuer string) (string, error) {
	thumb, err := k.Thumbprint()
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims{
		"aud":       issuer + "/keys",
		"exp":       now.Add(5 * time.Minute).Unix(),
		"iat":       now.Unix(),
		"jwk_thumb": thumb,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["typ"] = ProofType
	return token.SignedString(k.private)
}
