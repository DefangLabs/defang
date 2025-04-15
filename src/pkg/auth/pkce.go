package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

type Method string

const (
	PlainMethod Method = "plain"
	S256Method  Method = "S256"
)

func generateVerifier(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func generateChallenge(verifier string, method Method) string {
	if method == PlainMethod {
		return verifier
	}
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

type PKCE struct {
	Verifier  string
	Challenge string
	Method
}

func GeneratePKCE(length int) (PKCE, error) {
	if length < 43 || length > 128 {
		return PKCE{}, errors.New(
			"code verifier length must be between 43 and 128 characters",
		)
	}
	verifier, err := generateVerifier(length)
	if err != nil {
		return PKCE{}, err
	}
	const method = S256Method
	challenge := generateChallenge(verifier, method)
	return PKCE{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    method,
	}, nil
}

func ValidatePKCE(
	verifier string,
	challenge string,
	method Method,
) bool {
	generatedChallenge := generateChallenge(verifier, method)
	// timing safe equals?
	return generatedChallenge == challenge
}
