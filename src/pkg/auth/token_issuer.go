package auth

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/nats-io/nats.go"
)

var (
	signingMethod = jwt.SigningMethodEdDSA
)

const (
	maxTokenLen = 300 // avoid DoS attacks by limiting the size of the tokens
)

type TokenIssuer struct {
	km          *KeyManager
	issuer      string
	revocations nats.KeyValue
}

func NewTokenIssuer(km *KeyManager, issuer string, revocations nats.KeyValue) *TokenIssuer {
	return &TokenIssuer{issuer: issuer, km: km, revocations: revocations}
}

func (ti *TokenIssuer) CreateAccessToken(sub string, dur time.Duration, aud ...string) string {
	if len(sub) == 0 {
		return "" // empty subject is not allowed
	}
	claims := jwt.RegisteredClaims{
		// ID:		jwt.NewUUID(), TODO: needed for revocation list?
		Subject:   sub,
		Issuer:    ti.issuer,
		Audience:  aud,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(dur)),
	}
	token := jwt.NewWithClaims(signingMethod, claims)
	current := ti.km.GetCurrentKey()
	token.Header["kid"] = current.Kid
	// Sign and get the complete encoded token as a string using the key
	tokenString, err := token.SignedString(current.PrivateKey)
	if err != nil {
		log.Println("failed to sign token:", err)
	}
	return tokenString
}

func (ti *TokenIssuer) verifyToken(token string, aud string) (string, error) {
	// Avoid DoS attacks by limiting the size of the tokens
	if len(token) > maxTokenLen {
		return "", jwt.ErrTokenMalformed
	}
	claims := jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(token, &claims, ti.lookupPubKey, jwt.WithValidMethods([]string{signingMethod.Alg()}))
	if err != nil {
		return "", err
	}
	if !claims.VerifyIssuer(ti.issuer, true) {
		return "", jwt.ErrTokenInvalidIssuer
	}
	if !claims.VerifyAudience(aud, false) {
		return "", jwt.ErrTokenInvalidAudience
	}
	return claims.Subject, nil
}

func (ti *TokenIssuer) VerifyAccessToken(token string, aud string) (string, error) {
	sub, err := ti.verifyToken(token, aud)
	if err != nil {
		return "", err
	}
	if err := ti.checkRevocationList(token); err != nil {
		return "", err
	}
	return sub, nil
}

func getTokenID(token string) string {
	// Use the signature part of the JWT to avoid storing PII TODO: could use claims.ID if set
	lastIndex := strings.LastIndex(token, ".")
	return token[lastIndex+1:]
}

func (ti *TokenIssuer) Revoke(token, reason string) error {
	_, err := ti.verifyToken(token, "")
	if err != nil {
		return err
	}
	if _, err = ti.revocations.Create(getTokenID(token), []byte(reason)); err != nil {
		if err != nats.ErrKeyExists {
			log.Println("failed to create revocation:", err)
			return errors.New("failed to create revocation")
		}
		return errors.New("token revoked")
	}
	return nil
}

func (ti *TokenIssuer) checkRevocationList(token string) error {
	if kve, err := ti.revocations.Get(getTokenID(token)); err != nats.ErrKeyNotFound {
		if err != nil {
			log.Println("failed to check revocation list:", err)
			return errors.New("failed to check revocation list")
		}
		return fmt.Errorf("token revoked: %s", string(kve.Value()))
	}
	return nil
}

func (ti *TokenIssuer) lookupPubKey(token *jwt.Token) (interface{}, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, errors.New("invalid KID")
	}
	key := ti.km.GetPublicKey(kid)
	if key == nil {
		return nil, errors.New("unknown KID; wrong cluster?")
	}
	return key, nil
}
