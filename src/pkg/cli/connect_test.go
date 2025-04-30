package cli

import (
	"context"
	"net/http"
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
	t.Parallel()
	ctx := context.Background()

	t.Run("unreachable", func(t *testing.T) {
		t.Skip("unreachable test only triggers when the wifi is off")
		_, err := Connect(ctx, "1.2.3.4")
		if expected, actual := "unavailable: dial tcp 1.2.3.4:443: connect: network is unreachable", err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping slow test in short mode")
		}
		// Use a non-routable IP address to trigger a timeout
		_, err := Connect(ctx, "240.0.0.1")
		if expected, actual := `deadline_exceeded: Post "https://240.0.0.1:443/io.defang.v1.FabricController/WhoAmI": dial tcp 240.0.0.1:443: i/o timeout`, err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		_, err := Connect(ctx, "127.0.0.1:1234")
		if expected, actual := "unavailable: dial tcp 127.0.0.1:1234: connect: connection refused", err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
	})

	t.Run("no such host", func(t *testing.T) {
		_, err := Connect(ctx, "blah.example.com")
		const suffix = ": no such host"
		if actual := err.Error(); !strings.HasSuffix(actual, suffix) {
			t.Errorf("expected error to end with %q, got: %v", suffix, actual)
		}
	})

	t.Run("unexpected EOF", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(server.Close)

		_, err := Connect(ctx, strings.TrimPrefix(server.URL, "http://"))
		if expected, actual := "internal: protocol error: no Grpc-Status trailer: unexpected EOF", err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
	})

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
