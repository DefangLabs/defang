package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"golang.org/x/oauth2/google"
)

var FindGoogleDefaultCredentials func(ctx context.Context, scopes ...string) (*google.Credentials, error) = google.FindDefaultCredentials

func (gcp Gcp) GetCurrentAccountEmail(ctx context.Context) (string, error) {
	creds, err := FindGoogleDefaultCredentials(ctx)
	if err != nil {
		return "", err
	}
	content := struct {
		ClientEmail string `json:"client_email"`
	}{}

	json.Unmarshal(creds.JSON, &content)
	if content.ClientEmail != "" {
		return content.ClientEmail, nil
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve token: %w", err)
	}
	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", errors.New("failed to retrieve ID token")
	}

	// Split the ID token into its 3 parts: header, payload, and signature
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		log.Fatalf("invalid ID token format")
	}

	// Decode and Parse the payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		log.Fatalf("failed to decode payload: %v", err)
	}
	claims := struct {
		Email string `json:"email"`
	}{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		log.Fatalf("failed to unmarshal payload: %v", err)
	}

	if claims.Email != "" {
		return claims.Email, nil
	}

	return "", nil // Should this be an error?
}
