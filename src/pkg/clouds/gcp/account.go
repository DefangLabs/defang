package gcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var FindGoogleDefaultCredentials func(ctx context.Context, scopes ...string) (*google.Credentials, error) = google.FindDefaultCredentials

// getEmailFromMetadataServer returns the email of the service account this
// workload runs as, read from the GCE metadata server. It is the canonical
// identity of a keyless workload on Compute Engine, where the ambient access
// token carries no email in tokeninfo (it lacks the userinfo.email scope).
// A package var so tests can stub it; it errors when not running on GCE.
var getEmailFromMetadataServer = func(ctx context.Context) (string, error) {
	if !metadata.OnGCE() {
		return "", errors.New("not running on GCE")
	}
	return metadata.EmailWithContext(ctx, "default")
}

// TODO: Possibly need to support google groups and domains type of principals
// Currently we only support:
// - Google Accounts (user:email)
// - Service Accounts (serviceAccount:xxx@xxx.gserviceaccount.com)
// - Principal Sets, i.e. Workload Identity Federation (principalSet:...)
//
// Whole list of possible principal types:
// https://cloud.google.com/iam/docs/principals-overview#principal-types
func (gcp Gcp) GetCurrentPrincipal(ctx context.Context) (string, error) {
	var token *oauth2.Token
	if gcp.Principal != "" {
		return gcp.Principal, nil
	}
	if gcp.TokenSource != nil {
		var err error
		token, err = gcp.TokenSource.Token()
		if err != nil {
			return "", fmt.Errorf("failed to retrieve token from provided TokenSource: %w", err)
		}
	} else {
		creds, err := FindGoogleDefaultCredentials(ctx)
		if err != nil {
			return "", err
		}

		// Unmarshal creds.JSON into a struct that includes both possible fields
		var key struct {
			ClientEmail                    string `json:"client_email"`
			Type                           string `json:"type"`
			Audience                       string `json:"audience"`
			ServiceAccountImpersonationURL string `json:"service_account_impersonation_url"`
		}
		err = json.Unmarshal(creds.JSON, &key)
		if err == nil {
			if key.Type == "external_account" {
				return removeProvider("principalSet:" + key.Audience), nil
			}
			if key.Type == "impersonated_service_account" {
				serviceAccount, err := parseServiceAccountFromURL(key.ServiceAccountImpersonationURL)
				if err != nil {
					return "", err
				}
				return "serviceAccount:" + serviceAccount, nil
			}
			if key.ClientEmail != "" {
				return getPrincipalFromEmail(key.ClientEmail), nil
			}
		}

		// Fallback: get token and try to extract email
		token, err = creds.TokenSource.Token()
		if err != nil {
			return "", fmt.Errorf("failed to retrieve token: %w", err)
		}
	}

	// Try to extract email from id_token if present
	if idToken, ok := token.Extra("id_token").(string); ok && idToken != "" {
		if email, err := extractEmailFromIDToken(idToken); err == nil && email != "" {
			return getPrincipalFromEmail(email), nil
		}
	}

	// Next: ask the tokeninfo endpoint for the token's email (works for user
	// and OAuth tokens minted with the email scope).
	email, err := getEmailFromToken(ctx, token.AccessToken)
	if err == nil {
		return getPrincipalFromEmail(email), nil
	}

	// Last resort: on GCE the ambient access token has no email in tokeninfo,
	// so read the instance service account's email from the metadata server.
	// This is what lets a keyless workload (e.g. a container deploying itself
	// via its attached service account) resolve its own principal.
	if mdEmail, mdErr := getEmailFromMetadataServer(ctx); mdErr == nil && mdEmail != "" {
		return getPrincipalFromEmail(mdEmail), nil
	}
	return "", fmt.Errorf("failed to get email from token: %w", err)
}

func extractEmailFromIDToken(idToken string) (string, error) {
	// JWT format: header.payload.signature
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", errors.New("invalid id_token format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode id_token payload: %w", err)
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("failed to unmarshal id_token claims: %w", err)
	}

	return claims.Email, nil
}

func getPrincipalFromEmail(email string) string {
	if strings.HasSuffix(email, ".gserviceaccount.com") {
		return "serviceAccount:" + email
	}
	return "user:" + email
}

func parseServiceAccountFromURL(url string) (string, error) {
	// URL format: https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/EMAIL:generateAccessToken
	re := regexp.MustCompile(`serviceAccounts/([^:]+):`)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		return matches[1], nil
	}
	return "", fmt.Errorf("unable to parse service account from URL: %s", url)
}

// getEmailFromToken queries Google's tokeninfo endpoint for a token's email.
// A package var so tests can exercise the metadata fallback without a network
// round-trip.
var getEmailFromToken = func(ctx context.Context, accessToken string) (string, error) {
	url := "https://www.googleapis.com/oauth2/v1/tokeninfo?access_token=" + accessToken

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenInfo struct {
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		return "", err
	}

	if tokenInfo.Email == "" {
		return "", errors.New("no email found in token info")
	}

	return tokenInfo.Email, nil
}

func removeProvider(principalSet string) string {
	// Find the position of "/providers/"
	providersIndex := strings.Index(principalSet, "/providers/")
	if providersIndex == -1 {
		// No providers path, return as-is
		return principalSet
	}

	// Extract everything before "/providers/" and append "/*"
	base := principalSet[:providersIndex]
	return base + "/*"
}
