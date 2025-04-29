package cli

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestConnect(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		handler := mockWhoAmI{}
		_, h := defangv1connect.NewFabricControllerHandler(handler)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)

		g, err := Connect(ctx, strings.TrimPrefix(server.URL, "http://"))
		if err != nil {
			t.Fatalf("expected %v, got: %v", nil, err)
		}
		if g == nil {
			t.Error("expected non-nil")
		}
	})

	t.Run("success ignore tenant", func(t *testing.T) {
		const expected = "tenant1"
		handler := mockWhoAmI{tenant: expected}
		_, h := defangv1connect.NewFabricControllerHandler(handler)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)

		// Try to override the tenant, but doesn't match the one from the "token"
		g, err := Connect(ctx, "ignored@"+strings.TrimPrefix(server.URL, "http://"))
		if err != nil {
			t.Fatalf("expected %v, got: %v", nil, err)
		}
		if g.TenantName != expected {
			t.Errorf("expected %v, got: %v", expected, g.TenantName)
		}
	})

	t.Run("success tenant from header", func(t *testing.T) {
		handler := mockWhoAmI{}
		_, h := defangv1connect.NewFabricControllerHandler(handler)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)

		// Try to override the tenant
		const expected = "tenant2"
		g, err := Connect(ctx, expected+"@"+strings.TrimPrefix(server.URL, "http://"))
		if err != nil {
			t.Fatalf("expected %v, got: %v", nil, err)
		}
		if g.TenantName != expected {
			t.Errorf("expected %v, got: %v", expected, g.TenantName)
		}
	})
}

type mockWhoAmI struct {
	tenant string
	defangv1connect.FabricControllerHandler
}

func (m mockWhoAmI) WhoAmI(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	tenant := m.tenant
	if tenant == "" {
		tenant = req.Header().Get(auth.XDefangOrgID)
	}
	return connect.NewResponse(&defangv1.WhoAmIResponse{Tenant: tenant}), nil
}
