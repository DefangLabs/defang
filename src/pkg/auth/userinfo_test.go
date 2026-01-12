package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUserInfo(t *testing.T) {
	var capturedAuth string
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		if capturedPath != "/userinfo" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"allTenants":[{"id":"t1","name":"Workspace One"}],
			"userinfo":{"email":"user@example.com","name":"Test User"}
		}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient("defang-cli", server.URL)

	info, err := client.UserInfo(t.Context(), "token-123")
	if capturedPath != "/userinfo" {
		t.Fatalf("unexpected path %q", capturedPath)
	}

	if err != nil {
		t.Fatalf("FetchUserInfo returned error: %v", err)
	}

	if capturedAuth != "Bearer token-123" {
		t.Fatalf("expected authorization header to be set, got %q", capturedAuth)
	}

	if len(info.AllTenants) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(info.AllTenants))
	}
	if info.AllTenants[0].ID != "t1" || info.AllTenants[0].Name != "Workspace One" {
		t.Fatalf("unexpected workspace data: %+v", info.AllTenants[0])
	}
	if info.User.Email != "user@example.com" || info.User.Name != "Test User" {
		t.Fatalf("unexpected user info: %+v", info.User)
	}
}

func TestFetchUserInfoErrors(t *testing.T) {
	t.Run("no access token", func(t *testing.T) {
		if _, err := OpenAuthClient.UserInfo(t.Context(), ""); err == nil {
			t.Fatalf("expected error for empty token")
		}
	})

	t.Run("invalid token (unauthorized)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(server.Close)
		openAuthClient := NewClient("defang-cli", server.URL)

		if _, err := openAuthClient.UserInfo(t.Context(), "token"); err == nil {
			t.Fatalf("expected error for non-200 response")
		}
	})

	t.Run("invalid response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{invalid json`))
		}))
		t.Cleanup(server.Close)
		client := NewClient("defang-cli", server.URL)

		if _, err := client.UserInfo(t.Context(), "token"); err == nil {
			t.Fatalf("expected error for malformed JSON")
		}
	})
}
