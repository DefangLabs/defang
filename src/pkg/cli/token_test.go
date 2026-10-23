package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type tokenClient struct {
	client.MockFabricClient
	request  *defangv1.TokenRequest
	response *defangv1.TokenResponse
	err      error
}

func (c *tokenClient) Token(_ context.Context, req *defangv1.TokenRequest) (*defangv1.TokenResponse, error) {
	c.request = req
	return c.response, c.err
}

func TestTokenWithAssertion(t *testing.T) {
	tests := []struct {
		name       string
		scope      scope.Scope
		wantScopes []string
	}{
		{name: "admin is unrestricted", scope: scope.Admin},
		{name: "limited scope", scope: scope.Read, wantScopes: []string{"read"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fabric := &tokenClient{response: &defangv1.TokenResponse{AccessToken: "defang_generated"}}
			const duration = 30 * 24 * time.Hour

			got, err := Token(t.Context(), fabric, types.TenantNameOrID("workspace-id"), duration, tt.scope, "login-jwt")
			if err != nil {
				t.Fatalf("Token() error = %v", err)
			}
			if got != "defang_generated" {
				t.Fatalf("Token() = %q, want %q", got, "defang_generated")
			}
			if fabric.request == nil {
				t.Fatal("Token() did not call Fabric Token")
			}
			if fabric.request.Assertion != "login-jwt" {
				t.Errorf("assertion = %q, want %q", fabric.request.Assertion, "login-jwt")
			}
			if fabric.request.Tenant != "workspace-id" {
				t.Errorf("tenant = %q, want %q", fabric.request.Tenant, "workspace-id")
			}
			if fabric.request.ExpiresIn != uint32(duration.Seconds()) {
				t.Errorf("expires_in = %d, want %d", fabric.request.ExpiresIn, uint32(duration.Seconds()))
			}
			if len(fabric.request.Scope) != len(tt.wantScopes) {
				t.Fatalf("scopes = %v, want %v", fabric.request.Scope, tt.wantScopes)
			}
			for i := range tt.wantScopes {
				if fabric.request.Scope[i] != tt.wantScopes[i] {
					t.Errorf("scopes = %v, want %v", fabric.request.Scope, tt.wantScopes)
				}
			}
		})
	}
}

func TestTokenWithAssertionReturnsFabricError(t *testing.T) {
	wantErr := errors.New("mint failed")
	fabric := &tokenClient{err: wantErr}

	_, err := Token(t.Context(), fabric, types.TenantNameOrID("workspace-id"), time.Hour, scope.Read, "login-jwt")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Token() error = %v, want %v", err, wantErr)
	}
}
