package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/defang-io/defang/src/internal/util_test"
	"github.com/golang-jwt/jwt/v4"
	natsServer "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
)

const jwtIssuer = "https://dummy"

func TestCreateVerify(t *testing.T) {
	ti := NewTokenIssuer(NewKeyManager(util_test.NewDummyKV()), jwtIssuer, util_test.NewDummyKV())

	var tok string

	t.Run("CreateAccessToken", func(t *testing.T) {
		tok = ti.CreateAccessToken("user", time.Second)
		if tok == "" {
			t.Fatal("expected JWT")
		}
	})

	t.Run("CreateAccessTokenEmptySubject", func(t *testing.T) {
		tok := ti.CreateAccessToken("", time.Second)
		if tok != "" {
			t.Fatal("expected empty JWT")
		}
	})

	t.Run("VerifyAccessToken", func(t *testing.T) {
		sub, err := ti.VerifyAccessToken(tok, "")
		if err != nil {
			t.Fatal(err)
		}
		if sub != "user" {
			t.Fatal("expected subject: user")
		}
	})

	t.Run("VertifyAccessTokenExpiry", func(t *testing.T) {
		token := ti.CreateAccessToken("foo", 0)
		_, err := ti.VerifyAccessToken(token, "")
		if !errors.Is(err, jwt.ErrTokenExpired) {
			t.Fatal(err)
		}
	})

	t.Run("VertifyAccessTokenMalformed", func(t *testing.T) {
		_, err := ti.VerifyAccessToken("token", "")
		if !errors.Is(err, jwt.ErrTokenMalformed) {
			t.Fatal(err)
		}
	})

	t.Run("VertifyAccessTokenMissingKID", func(t *testing.T) {
		token := jwt.New(signingMethod)
		tokenString, err := token.SignedString(ti.km.getCurrentKey().PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ti.VerifyAccessToken(tokenString, "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("VertifyAccessTokenInvalidKID", func(t *testing.T) {
		// Make a JWT with an invalid KID
		token := jwt.New(signingMethod)
		token.Header["kid"] = "bogus"
		tokenString, err := token.SignedString(ti.km.getCurrentKey().PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ti.VerifyAccessToken(tokenString, "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("VertifyAccessTokenInvalidIssuer", func(t *testing.T) {
		// Make a JWT with an invalid issuer
		claims := jwt.RegisteredClaims{
			Issuer: "bogus",
		}
		token := jwt.NewWithClaims(signingMethod, claims)
		token.Header["kid"] = ti.km.getCurrentKey().Kid
		tokenString, err := token.SignedString(ti.km.getCurrentKey().PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		_, err = ti.VerifyAccessToken(tokenString, "")
		if !errors.Is(err, jwt.ErrTokenInvalidIssuer) {
			t.Fatal(err)
		}
	})

	t.Run("VerifyAccessToken w/ aud", func(t *testing.T) {
		_, err := ti.VerifyAccessToken(tok, "http://audience")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Revoke", func(t *testing.T) {
		if err := ti.Revoke(tok, "logout"); err != nil {
			t.Fatal(err)
		}
		if _, err := ti.VerifyAccessToken(tok, ""); err == nil || err.Error() != "token revoked: logout" {
			t.Errorf("expected revoked error, got: %v", err)
		}
	})

	t.Run("Revoke twice", func(t *testing.T) {
		if err := ti.Revoke(tok, "logout"); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("Generate with audience", func(t *testing.T) {
		tok = ti.CreateAccessToken("user", time.Second, "http://audience")
		_, err := ti.VerifyAccessToken(tok, "http://audience")
		if err != nil {
			t.Fatal(err)
		}
		_, err = ti.VerifyAccessToken(tok, "")
		if err == nil {
			t.Fatal("expected error")
		}
		_, err = ti.VerifyAccessToken(tok, "http://audience2")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRevocationsWithRealNats(t *testing.T) {
	opts := natsServer.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natsServer.RunServer(&opts)
	defer s.Shutdown()

	nc, _ := nats.Connect(s.ClientURL())
	defer nc.Close()
	js, _ := nc.JetStream()
	kv, _ := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:  "keys",
		Storage: nats.MemoryStorage,
		History: nats.KeyValueMaxHistory,
	})

	km := NewKeyManager(kv)
	ti := NewTokenIssuer(km, jwtIssuer, kv)

	tok := ti.CreateAccessToken("user", time.Second)
	if _, err := ti.VerifyAccessToken(tok, ""); err != nil {
		t.Fatal(err)
	}
	if err := ti.Revoke(tok, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ti.VerifyAccessToken(tok, ""); err == nil {
		t.Fatal("expected error")
	}
}
