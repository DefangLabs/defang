package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"golang.org/x/oauth2"
)

func marshalToken(t *testing.T, tok oauth2.Token) string {
	t.Helper()
	b, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshaling token: %v", err)
	}
	return string(b)
}

func TestFindStoredCredentials_GCP(t *testing.T) {
	t.Run("nil token store returns nil", func(t *testing.T) {
		gcp := &Gcp{}
		ts, err := gcp.findStoredCredentials(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts != nil {
			t.Error("expected nil token source for nil TokenStore")
		}
	})

	t.Run("empty store returns nil", func(t *testing.T) {
		gcp := &Gcp{TokenStore: tokenstore.NewMemTokenStore()}
		ts, err := gcp.findStoredCredentials(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts != nil {
			t.Error("expected nil token source for empty store")
		}
	})

	t.Run("list error propagates", func(t *testing.T) {
		store := tokenstore.NewMemTokenStore()
		store.ListErr = errors.New("disk failure")
		gcp := &Gcp{TokenStore: store}
		_, err := gcp.findStoredCredentials(t.Context())
		if err == nil {
			t.Error("expected error from List failure")
		}
	})

	t.Run("invalid JSON is skipped", func(t *testing.T) {
		store := tokenstore.NewMemTokenStore()
		store.Save("bad-token", "not-json") //nolint:errcheck
		gcp := &Gcp{TokenStore: store}
		ts, err := gcp.findStoredCredentials(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts != nil {
			t.Error("expected nil: bad token should be skipped")
		}
	})

	t.Run("token without permissions is skipped", func(t *testing.T) {
		orig := testTokenProjectPermissions
		testTokenProjectPermissions = func(_ context.Context, _ string, _ []string, _ oauth2.TokenSource) error {
			return errors.New("missing permissions")
		}
		t.Cleanup(func() { testTokenProjectPermissions = orig })

		store := tokenstore.NewMemTokenStore()
		store.Save("user@example.com", marshalToken(t, oauth2.Token{ //nolint:errcheck
			AccessToken: "tok",
			Expiry:      time.Now().Add(time.Hour),
		}))
		gcp := &Gcp{TokenStore: store}
		ts, err := gcp.findStoredCredentials(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts != nil {
			t.Error("expected nil: token without permissions should be skipped")
		}
	})

	t.Run("valid token with permissions returns token source", func(t *testing.T) {
		orig := testTokenProjectPermissions
		testTokenProjectPermissions = func(_ context.Context, _ string, _ []string, _ oauth2.TokenSource) error {
			return nil // all permissions granted
		}
		t.Cleanup(func() { testTokenProjectPermissions = orig })

		tok := oauth2.Token{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			Expiry:       time.Now().Add(time.Hour),
		}
		store := tokenstore.NewMemTokenStore()
		store.Save("user@example.com", marshalToken(t, tok)) //nolint:errcheck
		gcp := &Gcp{TokenStore: store}
		ts, err := gcp.findStoredCredentials(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts == nil {
			t.Error("expected non-nil token source")
		}
	})

	t.Run("multiple tokens: first with permissions wins", func(t *testing.T) {
		orig := testTokenProjectPermissions
		calls := 0
		testTokenProjectPermissions = func(_ context.Context, _ string, _ []string, _ oauth2.TokenSource) error {
			calls++
			return nil // all pass
		}
		t.Cleanup(func() { testTokenProjectPermissions = orig })

		store := tokenstore.NewMemTokenStore()
		tok := oauth2.Token{AccessToken: "tok", Expiry: time.Now().Add(time.Hour)}
		store.Save("user-a@example.com", marshalToken(t, tok)) //nolint:errcheck
		store.Save("user-b@example.com", marshalToken(t, tok)) //nolint:errcheck
		gcp := &Gcp{TokenStore: store}
		ts, err := gcp.findStoredCredentials(t.Context())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts == nil {
			t.Error("expected non-nil token source")
		}
		if calls != 1 {
			t.Errorf("expected 1 permission check (stop after first match), got %d", calls)
		}
	})
}

func TestParseWIFProvider(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProject  string
		wantPool     string
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "valid provider string",
			input:        "//iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/defang-github/providers/github-actions",
			wantProject:  "123456789012",
			wantPool:     "defang-github",
			wantProvider: "github-actions",
		},
		{
			name:         "valid provider with suffix",
			input:        "//iam.googleapis.com/projects/my-project/locations/global/workloadIdentityPools/my-pool/providers/my-provider",
			wantProject:  "my-project",
			wantPool:     "my-pool",
			wantProvider: "my-provider",
		},
		{
			name:    "too few segments",
			input:   "//iam.googleapis.com/projects/123/locations/global",
			wantErr: true,
		},
		{
			name:    "wrong keyword workloadIdentityPools",
			input:   "//iam.googleapis.com/projects/123/locations/global/wrongKeyword/pool/providers/provider",
			wantErr: true,
		},
		{
			name:    "wrong keyword providers",
			input:   "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/wrongKeyword/provider",
			wantErr: true,
		},
		{
			name:    "wrong keyword projects",
			input:   "//iam.googleapis.com/wrongKeyword/123/locations/global/workloadIdentityPools/pool/providers/provider",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, pool, provider, err := parseWIFProvider(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWIFProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if project != tt.wantProject {
					t.Errorf("project = %q, want %q", project, tt.wantProject)
				}
				if pool != tt.wantPool {
					t.Errorf("pool = %q, want %q", pool, tt.wantPool)
				}
				if provider != tt.wantProvider {
					t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
				}
			}
		})
	}
}
