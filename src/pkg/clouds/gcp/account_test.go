package gcp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type MockTokenSource struct {
	token *oauth2.Token
}

func (m *MockTokenSource) Token() (*oauth2.Token, error) {
	return m.token, nil
}

func TestGetCurrentAccountEmail(t *testing.T) {
	t.Run("Email in credentials", func(t *testing.T) {
		FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON: []byte(`{"client_email":"test@email.com"}`),
			}, nil
		}
		var gcp Gcp
		email, err := gcp.GetCurrentAccountEmail(t.Context())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expected := "test@email.com"
		if email != expected {
			t.Errorf("expected email to be %s, got %s", expected, email)
		}
	})

	t.Run("Email in refreshed token", func(t *testing.T) {
		token := &oauth2.Token{}
		token = token.WithExtra(map[string]any{
			"id_token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJ1bml0IHRlc3QiLCJpYXQiOm51bGwsImV4cCI6bnVsbCwiYXVkIjoiIiwic3ViIjoiIiwiZW1haWwiOiJ0ZXN0QGVtYWlsLmNvbSJ9.UP2OF86aOg2BkpbFrkKUQ-osrwhTjh9_2JOnUlGMmHM",
		})
		FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON:        []byte(``),
				TokenSource: &MockTokenSource{token: token},
			}, nil
		}
		var gcp Gcp
		email, err := gcp.GetCurrentAccountEmail(t.Context())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expected := "test@email.com"
		if email != expected {
			t.Errorf("expected email to be %s, got %s", expected, email)
		}
	})

	t.Run("External account with service account impersonation", func(t *testing.T) {
		FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON: []byte(`{
				"type": "external_account",
				"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/1234567890/serviceAccounts/test@project.iam.gserviceaccount.com:generateAccessToken"
			}`),
			}, nil
		}
		var gcp Gcp
		email, err := gcp.GetCurrentAccountEmail(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expected := "test@project.iam.gserviceaccount.com"
		if email != expected {
			t.Errorf("expected email to be %s, got %s", expected, email)
		}
	})

	t.Run("Error getting credentials", func(t *testing.T) {
		FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return nil, errors.New("no credentials found")
		}
		var gcp Gcp
		_, err := gcp.GetCurrentAccountEmail(context.Background())
		if err == nil {
			t.Error("expected error but got none")
		}
	})

	t.Run("Token error", func(t *testing.T) {
		FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON:        []byte(`{}`),
				TokenSource: &MockTokenSourceWithError{},
			}, nil
		}
		var gcp Gcp
		_, err := gcp.GetCurrentAccountEmail(context.Background())
		if err == nil {
			t.Error("expected error but got none")
		}
	})

	t.Run("Invalid JSON in credentials", func(t *testing.T) {
		token := &oauth2.Token{AccessToken: "test-token"}
		FindGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON:        []byte(`invalid json`),
				TokenSource: &MockTokenSource{token: token},
			}, nil
		}
		var gcp Gcp
		_, err := gcp.GetCurrentAccountEmail(context.Background())
		if err == nil {
			t.Error("expected error but got none")
		}
	})
}

type MockTokenSourceWithError struct{}

func (m *MockTokenSourceWithError) Token() (*oauth2.Token, error) {
	return nil, errors.New("token error")
}

func TestExtractEmailFromIDToken(t *testing.T) {
	t.Run("Valid ID token", func(t *testing.T) {
		header := `{"typ":"JWT","alg":"HS256"}`
		payload := `{"email":"test@email.com"}`
		idToken := encodeJWT(header, payload)
		email, err := extractEmailFromIDToken(idToken)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expected := "test@email.com"
		if email != expected {
			t.Errorf("expected email to be %s, got %s", expected, email)
		}
	})

	t.Run("Invalid token format", func(t *testing.T) {
		idToken := "invalid.token"
		_, err := extractEmailFromIDToken(idToken)
		if err == nil {
			t.Error("expected error but got none")
		}
	})

	t.Run("Invalid base64 payload", func(t *testing.T) {
		idToken := "header.invalid_base64.signature"
		_, err := extractEmailFromIDToken(idToken)
		if err == nil {
			t.Error("expected error but got none")
		}
	})

	t.Run("Invalid JSON payload", func(t *testing.T) {
		idToken := encodeJWT("header", "invalid json")
		_, err := extractEmailFromIDToken(idToken)
		if err == nil {
			t.Error("expected error but got none")
		}
	})

	t.Run("Empty email in token", func(t *testing.T) {
		// JWT with payload: {"email":""}
		header := `{"typ":"JWT","alg":"HS256"}`
		payload := `{"email":""}`
		idToken := encodeJWT(header, payload)
		email, err := extractEmailFromIDToken(idToken)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if email != "" {
			t.Errorf("expected empty email, got %s", email)
		}
	})
}

func TestParseServiceAccountFromURL(t *testing.T) {
	t.Run("Valid service account URL", func(t *testing.T) {
		url := "https://iamcredentials.googleapis.com/v1/projects/123456789/serviceAccounts/test@project.iam.gserviceaccount.com:generateAccessToken"
		email, err := parseServiceAccountFromURL(url)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expected := "test@project.iam.gserviceaccount.com"
		if email != expected {
			t.Errorf("expected email to be %s, got %s", expected, email)
		}
	})

	t.Run("Invalid URL format", func(t *testing.T) {
		url := "https://invalidpath"
		_, err := parseServiceAccountFromURL(url)
		if err == nil {
			t.Error("expected error but got none")
		}
	})

	t.Run("URL without service account", func(t *testing.T) {
		url := "https://iamcredentials.googleapis.com/v1/projects/123456789/notaserviceaccount"
		_, err := parseServiceAccountFromURL(url)
		if err == nil {
			t.Error("expected error but got none")
		}
	})
}

func encodeJWT(header, payload string) string {
	encode := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	return fmt.Sprintf("%s.%s.signature", encode(header), encode(payload))
}
