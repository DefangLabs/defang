package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUserInfo(t *testing.T) {
	originalClient := openAuthClient
	t.Cleanup(func() { openAuthClient = originalClient })

	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/userinfo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"allTenants":[{"id":"t1","name":"Workspace One"}],
			"userinfo":{"email":"user@example.com","name":"Test User"}
		}`))
	}))
	defer server.Close()

	openAuthClient = NewClient("defang-cli", server.URL)

	info, err := FetchUserInfo(context.Background(), "token-123")
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
	originalClient := openAuthClient
	t.Cleanup(func() { openAuthClient = originalClient })

	if _, err := FetchUserInfo(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty token")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	openAuthClient = NewClient("defang-cli", server.URL)

	if _, err := FetchUserInfo(context.Background(), "token"); err == nil {
		t.Fatalf("expected error for non-200 response")
	}
}
