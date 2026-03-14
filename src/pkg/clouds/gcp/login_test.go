package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/iam/apiv1/iampb"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

// mockProjectsClient implements projectsClientIface for testing.
type mockProjectsClient struct {
	testIamPermissionsFunc func(ctx context.Context, req *iampb.TestIamPermissionsRequest, opts ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error)
}

func (m *mockProjectsClient) TestIamPermissions(ctx context.Context, req *iampb.TestIamPermissionsRequest, opts ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
	return m.testIamPermissionsFunc(ctx, req, opts...)
}

func (m *mockProjectsClient) Close() error { return nil }

// mockNewProjectsClient replaces newProjectsClient for the duration of a test,
// returning a client whose TestIamPermissions always calls fn.
func mockNewProjectsClient(t *testing.T, fn func(ctx context.Context, req *iampb.TestIamPermissionsRequest, opts ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error)) {
	t.Helper()
	orig := newProjectsClient
	newProjectsClient = func(ctx context.Context, opts ...option.ClientOption) (projectsClientIface, error) {
		return &mockProjectsClient{testIamPermissionsFunc: fn}, nil
	}
	t.Cleanup(func() { newProjectsClient = orig })
}

func marshalToken(t *testing.T, tok oauth2.Token) string {
	t.Helper()
	b, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshaling token: %v", err)
	}
	return string(b)
}

func allPermsGranted(_ context.Context, req *iampb.TestIamPermissionsRequest, _ ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
	return &iampb.TestIamPermissionsResponse{Permissions: req.Permissions}, nil
}

func noPermsGranted(_ context.Context, _ *iampb.TestIamPermissionsRequest, _ ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
	return &iampb.TestIamPermissionsResponse{}, nil
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
		mockNewProjectsClient(t, noPermsGranted)

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
		mockNewProjectsClient(t, allPermsGranted)

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
		calls := 0
		mockNewProjectsClient(t, func(ctx context.Context, req *iampb.TestIamPermissionsRequest, opts ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
			calls++
			return &iampb.TestIamPermissionsResponse{Permissions: req.Permissions}, nil
		})

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

func TestAuthenticate_GCP(t *testing.T) {
	t.Run("valid ADC credentials authenticate successfully", func(t *testing.T) {
		mockNewProjectsClient(t, allPermsGranted)

		gcp := &Gcp{ProjectId: "test-project"}
		if err := gcp.Authenticate(t.Context(), false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ADC missing perms, stored token valid", func(t *testing.T) {
		calls := 0
		mockNewProjectsClient(t, func(ctx context.Context, req *iampb.TestIamPermissionsRequest, opts ...gax.CallOption) (*iampb.TestIamPermissionsResponse, error) {
			calls++
			if calls == 1 {
				// First call: ADC check — return no permissions
				return &iampb.TestIamPermissionsResponse{}, nil
			}
			// Subsequent calls: stored token check — grant all permissions
			return &iampb.TestIamPermissionsResponse{Permissions: req.Permissions}, nil
		})

		tok := oauth2.Token{
			AccessToken:  "stored-token",
			RefreshToken: "refresh",
			Expiry:       time.Now().Add(time.Hour),
		}
		store := tokenstore.NewMemTokenStore()
		store.Save("user@example.com", marshalToken(t, tok)) //nolint:errcheck

		gcp := &Gcp{ProjectId: "test-project", TokenStore: store}
		if err := gcp.Authenticate(t.Context(), false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gcp.TokenSource == nil {
			t.Error("expected TokenSource to be set after finding stored credentials")
		}
	})

	t.Run("ADC missing perms, no stored credentials, non-interactive returns error", func(t *testing.T) {
		mockNewProjectsClient(t, noPermsGranted)

		gcp := &Gcp{ProjectId: "test-project", TokenStore: tokenstore.NewMemTokenStore()}
		err := gcp.Authenticate(t.Context(), false)
		if err == nil {
			t.Fatal("expected error for non-interactive with no valid credentials")
		}
	})

	t.Run("newProjectsClient error falls through to stored credentials", func(t *testing.T) {
		calls := 0
		orig := newProjectsClient
		newProjectsClient = func(ctx context.Context, opts ...option.ClientOption) (projectsClientIface, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("ADC unavailable")
			}
			return &mockProjectsClient{testIamPermissionsFunc: allPermsGranted}, nil
		}
		t.Cleanup(func() { newProjectsClient = orig })

		tok := oauth2.Token{
			AccessToken:  "stored-token",
			RefreshToken: "refresh",
			Expiry:       time.Now().Add(time.Hour),
		}
		store := tokenstore.NewMemTokenStore()
		store.Save("user@example.com", marshalToken(t, tok)) //nolint:errcheck

		gcp := &Gcp{ProjectId: "test-project", TokenStore: store}
		if err := gcp.Authenticate(t.Context(), false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gcp.TokenSource == nil {
			t.Error("expected TokenSource to be set after finding stored credentials")
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
