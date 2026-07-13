package identity

import (
	"crypto/rsa"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestLoadOrGenerateKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")

	generated, err := LoadOrGenerateKey(dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "private.pem"))
	if err != nil {
		t.Fatalf("stat private.pem: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("private.pem permissions = %o, want 0600", perm)
	}

	loaded, err := LoadOrGenerateKey(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	generatedThumb, err := generated.Thumbprint()
	if err != nil {
		t.Fatalf("thumbprint: %v", err)
	}
	loadedThumb, err := loaded.Thumbprint()
	if err != nil {
		t.Fatalf("thumbprint: %v", err)
	}
	if generatedThumb != loadedThumb {
		t.Errorf("loaded key thumbprint %q != generated %q", loadedThumb, generatedThumb)
	}
}

func TestLoadOrGenerateKeyInvalidPem(t *testing.T) {
	tests := []struct {
		name string
		pem  string
	}{
		{"not pem", "hello"},
		{"wrong key type", "-----BEGIN EC PRIVATE KEY-----\nAAAA\n-----END EC PRIVATE KEY-----\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "private.pem"), []byte(tt.pem), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadOrGenerateKey(dir); err == nil {
				t.Error("expected error for invalid private.pem, got nil")
			}
		})
	}
}

func TestPublicJWKIsMinimalRSA(t *testing.T) {
	key, err := LoadOrGenerateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := json.Marshal(key.PublicJWK())
	if err != nil {
		t.Fatal(err)
	}
	var jwk map[string]any
	if err := json.Unmarshal(bytes, &jwk); err != nil {
		t.Fatal(err)
	}
	if jwk["kty"] != "RSA" {
		t.Errorf("kty = %v, want RSA", jwk["kty"])
	}
	for _, private := range []string{"d", "p", "q", "dp", "dq", "qi"} {
		if _, leaked := jwk[private]; leaked {
			t.Errorf("public JWK leaks private field %q", private)
		}
	}
}

func TestPopJWT(t *testing.T) {
	key, err := LoadOrGenerateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	signed, err := key.PopJWT(now)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := jwt.Parse(signed, func(token *jwt.Token) (any, error) {
		public, ok := key.private.Public().(*rsa.PublicKey)
		if !ok {
			t.Fatal("not an RSA key")
		}
		return public, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Fatalf("PoP JWT does not verify with its own public key: %v", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims are %T, not MapClaims", parsed.Claims)
	}
	thumb, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	if claims["jwk_thumb"] != thumb {
		t.Errorf("jwk_thumb = %v, want %v", claims["jwk_thumb"], thumb)
	}
	if iat, _ := claims.GetIssuedAt(); iat == nil || iat.Unix() != now.Unix() {
		t.Errorf("iat = %v, want %v", iat, now.Unix())
	}
}
