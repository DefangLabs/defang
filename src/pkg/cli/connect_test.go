package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestConnect(t *testing.T) {
	ctx := t.Context()

	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		t.Skip("unreachable test only triggers when the wifi is off")
		_, err := Connect(ctx, "1.2.3.4")
		if expected, actual := "unavailable: dial tcp 1.2.3.4:443: connect: network is unreachable", err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
		if !client.IsNetworkError(err) {
			t.Errorf("expected network error, got: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		if testing.Short() {
			t.Skip("skipping slow test in short mode")
		}
		// Use a non-routable IP address to trigger a timeout
		_, err := Connect(ctx, "240.0.0.1")
		expected := []string{
			`deadline_exceeded: Post "https://240.0.0.1:443/io.defang.v1.FabricController/WhoAmI": dial tcp 240.0.0.1:443: i/o timeout`,
			`unavailable: dial tcp 240.0.0.1:443: connect: connection refused`,
		}
		if actual := err.Error(); !slices.Contains(expected, actual) {
			t.Errorf("expected one of %v, got: %v", expected, actual)
		}
		if !client.IsNetworkError(err) {
			t.Errorf("expected network error, got: %v", err)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		t.Parallel()
		_, err := Connect(ctx, "127.0.0.1:1234")
		if expected, actual := "unavailable: dial tcp 127.0.0.1:1234: connect: connection refused", err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
		if !client.IsNetworkError(err) {
			t.Errorf("expected network error, got: %v", err)
		}
	})

	t.Run("no such host", func(t *testing.T) {
		t.Parallel()
		_, err := Connect(ctx, "blah.example.com")
		suffixes := []string{": no such host", "device or resource busy"}
		if actual := err.Error(); !slices.ContainsFunc(suffixes, func(suffix string) bool { return strings.HasSuffix(actual, suffix) }) {
			t.Errorf("expected error to end with %q, got: %v", suffixes, actual)
		}
		if !client.IsNetworkError(err) {
			t.Errorf("expected network error, got: %v", err)
		}
	})

	t.Run("unexpected EOF", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(server.Close)

		_, err := Connect(ctx, strings.TrimPrefix(server.URL, "http://"))
		if expected, actual := "internal: protocol error: no Grpc-Status trailer: unexpected EOF", err.Error(); expected != actual {
			t.Errorf("expected %v, got: %v", expected, actual)
		}
		if !client.IsNetworkError(err) {
			t.Errorf("expected network error, got: %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		handler := &mockWhoAmI{}
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

	t.Run("success ignores server tenant response", func(t *testing.T) {
		t.Parallel()
		handler := &mockWhoAmI{tenant: "server-tenant", tenantID: "server-id"}
		_, h := defangv1connect.NewFabricControllerHandler(handler)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)

		g, err := Connect(ctx, strings.TrimPrefix(server.URL, "http://"))
		if err != nil {
			t.Fatalf("expected %v, got: %v", nil, err)
		}
		if g.GetTenantName() != types.TenantUnset {
			t.Errorf("expected tenant to remain unset, got: %v", g.GetTenantName())
		}
		if handler.seenTenant != "" {
			t.Errorf("expected empty tenant header, got: %q", handler.seenTenant) // default connection should not force a tenant header
		}
	})

	t.Run("explicit tenant header", func(t *testing.T) {
		t.Parallel()
		handler := &mockWhoAmI{tenant: "server-tenant", tenantID: "server-id"}
		_, h := defangv1connect.NewFabricControllerHandler(handler)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)

		const requested = "tenant2"
		g, err := ConnectWithTenant(ctx, strings.TrimPrefix(server.URL, "http://"), requested)
		if err != nil {
			t.Fatalf("expected %v, got: %v", nil, err)
		}
		if handler.seenTenant != requested {
			t.Errorf("expected header %q, got: %q", requested, handler.seenTenant)
		}
		if g.GetRequestedTenant() != requested {
			t.Errorf("expected requested tenant %q, got %q", requested, g.GetRequestedTenant())
		}
		if g.GetTenantName() != requested {
			t.Errorf("expected tenant %q, got %q", requested, g.GetTenantName())
		}
	})

	t.Run("legacy cluster prefix ignored", func(t *testing.T) {
		t.Parallel()
		handler := &mockWhoAmI{tenant: "server-tenant"}
		_, h := defangv1connect.NewFabricControllerHandler(handler)
		server := httptest.NewServer(h)
		t.Cleanup(server.Close)

		g, err := Connect(ctx, "ignored@"+strings.TrimPrefix(server.URL, "http://"))
		if err != nil {
			t.Fatalf("expected %v, got: %v", nil, err)
		}
		if handler.seenTenant != "" {
			t.Errorf("expected empty tenant header, got: %q", handler.seenTenant)
		}
		if g.GetTenantName() != types.TenantUnset {
			t.Errorf("expected tenant to remain unset, got: %v", g.GetTenantName())
		}
	})
}

type mockWhoAmI struct {
	tenant     string
	tenantID   string
	seenTenant string
	defangv1connect.UnimplementedFabricControllerHandler
}

func (m *mockWhoAmI) WhoAmI(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	m.seenTenant = req.Header().Get(auth.XDefangTenantID)
	return connect.NewResponse(&defangv1.WhoAmIResponse{Tenant: m.tenant, TenantId: m.tenantID}), nil
}
