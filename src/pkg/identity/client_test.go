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

func TestIssuerURL(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		tenant  string
		want    string
		wantErr bool
	}{
		{"default domain", "", "acme", "https://acme.id.defang.io", false},
		{"custom domain", "id.example.test", "acme", "https://acme.id.example.test", false},
		{"empty tenant", "", "", "", true},
		{"uppercase", "", "Acme", "", true},
		{"too short", "", "ab", "", true},
		{"double hyphen", "", "a--b", "", true},
		{"nested", "", "a.b", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IssuerURL(tt.domain, tt.tenant)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("IssuerURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// fakeService records which virtual host each request was meant for, the way
// id.defang.io routes on the tenant subdomain.
func fakeService(t *testing.T, handler func(host string, w http.ResponseWriter, r *http.Request)) {
	t.Helper()
	var host string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization = %q", auth)
		}
		handler(host, w, r)
	}))
	t.Cleanup(server.Close)
	original := baseURL
	baseURL = func(h string) string {
		host = h
		return server.URL
	}
	t.Cleanup(func() { baseURL = original })
}

func TestClaimTenant(t *testing.T) {
	fakeService(t, func(host string, w http.ResponseWriter, r *http.Request) {
		if host != "id.defang.io" || r.Method != http.MethodPost || r.URL.Path != "/tenants" {
			t.Errorf("unexpected request: %s %s %s", host, r.Method, r.URL.Path)
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(map[string]string{"name": body["name"], "issuer": "https://" + body["name"] + ".id.defang.io"})
	})
	tenant, err := NewClient("", "test-token").ClaimTenant(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Issuer != "https://acme.id.defang.io" {
		t.Errorf("tenant = %+v", tenant)
	}
}

func TestClientRegister(t *testing.T) {
	key, err := LoadOrGenerateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var gotBody map[string]any
	fakeService(t, func(host string, w http.ResponseWriter, r *http.Request) {
		if host != "acme.id.defang.io" || r.Method != http.MethodPost || r.URL.Path != "/keys" {
			t.Errorf("unexpected request: %s %s %s", host, r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"kid": "thumb123", "issuer": "https://acme.id.defang.io"})
	})

	proof, err := key.ProofJWT(time.Now(), "https://acme.id.defang.io")
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewClient("", "test-token").Register(context.Background(), "acme", RegisterRequest{
		JWK:   key.PublicJWK(),
		Proof: proof,
		Label: "laptop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Kid != "thumb123" || registered.Issuer != "https://acme.id.defang.io" {
		t.Errorf("unexpected response: %+v", registered)
	}
	if gotBody["label"] != "laptop" || gotBody["proof"] == "" {
		t.Errorf("request body = %v", gotBody)
	}
	if _, hasTTL := gotBody["ttl_seconds"]; hasTTL {
		t.Error("ttl_seconds should be omitted when zero")
	}
	if jwk, _ := gotBody["jwk"].(map[string]any); jwk["kty"] != "RSA" {
		t.Errorf("jwk = %v", gotBody["jwk"])
	}
}

func TestClientErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"error with message", http.StatusBadRequest, `{"error":"invalid_key","message":"RSA keys must be at least 2048 bits"}`, "invalid_key: RSA keys must be at least 2048 bits"},
		{"not found explains itself", http.StatusNotFound, `{"error":"not_found"}`, "you do not own it"},
		// 400, not 5xx: the retrying HTTP client would replay a 5xx for ~15s
		{"plain error body", http.StatusBadRequest, "boom", "unexpected status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeService(t, func(_ string, w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			})
			_, err := NewClient("", "test-token").List(context.Background(), "acme")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestClientListAndRevoke(t *testing.T) {
	fakeService(t, func(host string, w http.ResponseWriter, r *http.Request) {
		if host != "acme.id.defang.io" {
			t.Errorf("host = %s", host)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /keys":
			json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{
					{"kid": "k1", "alg": "RS256", "label": "laptop", "active": true, "created_at": "2026-10-28T00:00:00Z", "expires_at": nil, "revoked_at": nil},
				},
			})
		case "DELETE /keys/k1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	registryClient := NewClient("", "test-token")
	keys, err := registryClient.List(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Kid != "k1" || !keys[0].Active || keys[0].Label == nil || *keys[0].Label != "laptop" {
		t.Errorf("keys = %+v", keys)
	}
	if err := registryClient.Revoke(context.Background(), "acme", "k1"); err != nil {
		t.Fatal(err)
	}
}
