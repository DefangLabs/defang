package gcp

import (
	"context"
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
		email, err := gcp.GetCurrentAccountEmail(context.Background())
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
		email, err := gcp.GetCurrentAccountEmail(context.Background())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		expected := "test@email.com"
		if email != expected {
			t.Errorf("expected email to be %s, got %s", expected, email)
		}
	})
}
