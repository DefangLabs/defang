package dns

import (
	"context"
	"errors"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type mockFabricClient struct {
	ipResp    *defangv1.ResolveIPAddrResponse
	ipErr     error
	cnameResp *defangv1.ResolveCNAMEResponse
	cnameErr  error
	nsResp    *defangv1.ResolveNSResponse
	nsErr     error

	lastIPReq    *defangv1.ResolveIPAddrRequest
	lastCNAMEReq *defangv1.ResolveCNAMERequest
	lastNSReq    *defangv1.ResolveNSRequest
}

func (m *mockFabricClient) ResolveIPAddr(_ context.Context, req *defangv1.ResolveIPAddrRequest) (*defangv1.ResolveIPAddrResponse, error) {
	m.lastIPReq = req
	return m.ipResp, m.ipErr
}

func (m *mockFabricClient) ResolveCNAME(_ context.Context, req *defangv1.ResolveCNAMERequest) (*defangv1.ResolveCNAMEResponse, error) {
	m.lastCNAMEReq = req
	return m.cnameResp, m.cnameErr
}

func (m *mockFabricClient) ResolveNS(_ context.Context, req *defangv1.ResolveNSRequest) (*defangv1.ResolveNSResponse, error) {
	m.lastNSReq = req
	return m.nsResp, m.nsErr
}

func TestFabricResolverLookupIPAddr(t *testing.T) {
	t.Run("returns parsed IPs and forwards NSServer", func(t *testing.T) {
		m := &mockFabricClient{
			ipResp: &defangv1.ResolveIPAddrResponse{IpAddrs: []string{"1.2.3.4", "::1", "not-an-ip"}},
		}
		r := FabricResolver{Client: m, NSServer: "ns.example.com"}
		ips, err := r.LookupIPAddr(t.Context(), "example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ips) != 2 {
			t.Fatalf("expected 2 valid IPs, got %v", ips)
		}
		if m.lastIPReq.Domain != "example.com" || m.lastIPReq.NsServer != "ns.example.com" {
			t.Errorf("request mismatch: %+v", m.lastIPReq)
		}
	})

	t.Run("empty IPs returns ErrNoSuchHost", func(t *testing.T) {
		m := &mockFabricClient{ipResp: &defangv1.ResolveIPAddrResponse{}}
		r := FabricResolver{Client: m}
		if _, err := r.LookupIPAddr(t.Context(), "nx.example.com"); !errors.Is(err, ErrNoSuchHost) {
			t.Errorf("expected ErrNoSuchHost, got %v", err)
		}
	})

	t.Run("propagates RPC error", func(t *testing.T) {
		boom := errors.New("rpc boom")
		m := &mockFabricClient{ipErr: boom}
		r := FabricResolver{Client: m}
		if _, err := r.LookupIPAddr(t.Context(), "example.com"); err != boom {
			t.Errorf("expected rpc error, got %v", err)
		}
	})
}

func TestFabricResolverLookupCNAME(t *testing.T) {
	t.Run("returns cname", func(t *testing.T) {
		m := &mockFabricClient{cnameResp: &defangv1.ResolveCNAMEResponse{Cname: "alb.example.com"}}
		r := FabricResolver{Client: m}
		cname, err := r.LookupCNAME(t.Context(), "api.example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cname != "alb.example.com" {
			t.Errorf("got %q", cname)
		}
	})

	t.Run("empty cname returns ErrNoSuchHost", func(t *testing.T) {
		m := &mockFabricClient{cnameResp: &defangv1.ResolveCNAMEResponse{}}
		r := FabricResolver{Client: m}
		if _, err := r.LookupCNAME(t.Context(), "api.example.com"); !errors.Is(err, ErrNoSuchHost) {
			t.Errorf("expected ErrNoSuchHost, got %v", err)
		}
	})
}

func TestFabricResolverLookupNS(t *testing.T) {
	m := &mockFabricClient{nsResp: &defangv1.ResolveNSResponse{Hosts: []string{"ns1.example.com.", "ns2.example.com."}}}
	r := FabricResolver{Client: m}
	ns, err := r.LookupNS(t.Context(), "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ns) != 2 || ns[0].Host != "ns1.example.com." {
		t.Errorf("unexpected NS result: %+v", ns)
	}
}

func TestUseFabricResolver(t *testing.T) {
	t.Cleanup(func() {
		fabricClient = nil
		ResolverAt = DirectResolverAt
	})

	m := &mockFabricClient{ipResp: &defangv1.ResolveIPAddrResponse{IpAddrs: []string{"9.9.9.9"}}}
	UseFabricResolver(m)

	// RootResolver should now delegate to FabricResolver.
	ips, err := RootResolver{}.LookupIPAddr(t.Context(), "example.com")
	if err != nil {
		t.Fatalf("RootResolver.LookupIPAddr: %v", err)
	}
	if len(ips) != 1 || ips[0].IP.String() != "9.9.9.9" {
		t.Errorf("unexpected IPs: %v", ips)
	}

	// ResolverAt should return a FabricResolver bound to the NS.
	r := ResolverAt("ns1.example.com")
	if fr, ok := r.(FabricResolver); !ok || fr.NSServer != "ns1.example.com" {
		t.Errorf("ResolverAt did not return FabricResolver: %T %+v", r, r)
	}
}
