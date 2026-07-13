package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTenantURL(t *testing.T) {
	tests := []struct {
		name    string
		issuer  string
		tenant  string
		want    string
		wantErr bool
	}{
		{"basic", "https://auth.defang.io", "acme", "https://acme.auth.defang.io", false},
		{"strips path", "https://auth.defang.io/base", "acme", "https://acme.auth.defang.io", false},
		{"empty tenant", "https://auth.defang.io", "", "", true},
		{"no scheme", "auth.defang.io", "acme", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TenantURL(tt.issuer, tt.tenant)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("TenantURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientRegister(t *testing.T) {
	key, err := LoadOrGenerateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/keys" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization = %q", auth)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"kid":    "thumb123",
			"sub":    "defang:project:app:stack:prod",
			"issuer": "https://acme.auth.defang.io",
		})
	}))
	defer server.Close()

	registryClient := NewClient(server.URL, "test-token")
	popJwt, err := key.PopJWT(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	registered, err := registryClient.Register(context.Background(), RegisterRequest{
		ProjectID: "app",
		StackID:   "prod",
		JWK:       key.PublicJWK(),
		PopJWT:    popJwt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Kid != "thumb123" || registered.Subject != "defang:project:app:stack:prod" {
		t.Errorf("unexpected response: %+v", registered)
	}

	if gotBody["project_id"] != "app" || gotBody["stack_id"] != "prod" {
		t.Errorf("request body project/stack = %v/%v", gotBody["project_id"], gotBody["stack_id"])
	}
	if _, hasTtl := gotBody["ttl_seconds"]; hasTtl {
		t.Error("ttl_seconds should be omitted when zero")
	}
	jwk, _ := gotBody["jwk"].(map[string]any)
	if jwk["kty"] != "RSA" {
		t.Errorf("jwk = %v", gotBody["jwk"])
	}
	if gotBody["pop_jwt"] == "" {
		t.Error("missing pop_jwt")
	}
}

func TestClientErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"conflict with message", http.StatusConflict, `{"error":"key already registered for project \"other\""}`, "key already registered"},
		// 400, not 5xx: the retrying HTTP client would replay a 5xx for ~15s
		{"plain error body", http.StatusBadRequest, "boom", "unexpected status"},
		{"unauthorized", http.StatusUnauthorized, `{"error":"invalid access token"}`, "invalid access token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := NewClient(server.URL, "token").List(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientListAndRevoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /keys":
			json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{
					{"kid": "k1", "sub": "defang:project:app:stack:prod", "project_id": "app", "stack_id": "prod", "created": 1700000000},
				},
			})
		case "DELETE /keys/k1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	registryClient := NewClient(server.URL, "token")

	keys, err := registryClient.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Kid != "k1" || keys[0].ProjectID != "app" {
		t.Errorf("keys = %+v", keys)
	}

	if err := registryClient.Revoke(context.Background(), "k1"); err != nil {
		t.Errorf("revoke: %v", err)
	}
	if err := registryClient.Revoke(context.Background(), "missing"); err == nil {
		t.Error("expected error revoking unknown kid")
	}
}
