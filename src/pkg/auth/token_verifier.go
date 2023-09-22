package auth

import (
	"crypto"
	"errors"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/golang-jwt/jwt/v4"
)

type TokenVerifier struct {
	jwksURL      string
	validMethods []string
	iss          string
	cache        atomic.Value // map[string]*rsa.PublicKey
}

func NewTokenVerifier(jwksURL string, iss string, validMethods []string) *TokenVerifier {
	return &TokenVerifier{
		jwksURL:      jwksURL,
		iss:          iss,
		validMethods: validMethods,
	}
}

type defangClaims struct {
	GithubUsername string `json:"github-username,omitempty"`
}

type heimdallClaims struct {
	jwt.RegisteredClaims
	DefangClaims *defangClaims `json:"https://defang.io/jwt/claims,omitempty"`
}

func (tv *TokenVerifier) VerifyAssertion(token string) (string, error) {
	claims := heimdallClaims{}
	_, err := jwt.ParseWithClaims(token, &claims, tv.lookupPubKey, jwt.WithValidMethods(tv.validMethods))
	if err != nil {
		return "", err
	}
	if !claims.VerifyIssuer(tv.iss, true) {
		return "", jwt.ErrTokenInvalidIssuer
	}
	if claims.DefangClaims == nil || len(claims.DefangClaims.GithubUsername) == 0 {
		return "", errors.New("missing github-username claim")
	}
	// TODO: check revocation list
	return claims.DefangClaims.GithubUsername, err
}

func (tv *TokenVerifier) lookupPubKey(token *jwt.Token) (interface{}, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, errors.New("invalid KID")
	}

	cache, _ := tv.cache.Load().(map[string]crypto.PublicKey)
	pub, ok := cache[kid]
	if !ok {
		cache, err := getJwksMap(tv.jwksURL)
		if err != nil {
			return nil, err
		}
		tv.cache.Store(cache)
		pub, ok = cache[kid]
		if !ok {
			return nil, errors.New("unknown KID")
		}
	}
	return pub, nil
}

func getJwksMap(jwksURL string) (map[string]crypto.PublicKey, error) {
	jwks, err := GetJwks(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	cache := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, jwk := range jwks.Keys {
		pub, err := jwk.PublicKey()
		if err != nil {
			log.Printf("failed to parse public key: %v\n", err)
			continue
		}
		cache[jwk.Kid] = pub
	}
	return cache, nil
}
