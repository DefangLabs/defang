package dns

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestFindNSServer(t *testing.T) {
	t.Cleanup(func() {
		ResolverAt = DirectResolverAt
	})

	t.Run("NS server not exist on domain", func(t *testing.T) {
		ResolverAt = func(nsServer string) Resolver {
			if strings.Contains(nsServer, "root-servers.net") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: []string{"1.tld-servers.com", "2.tld-servers.com"}, Error: nil},
				}}
			} else if strings.Contains(nsServer, "tld-servers.com") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: []string{"1.dns-servers.com", "2.dns-servers.com"}, Error: nil},
				}}
			} else if strings.Contains(nsServer, "dns-servers.com") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: nil, Error: nil},
				}}
			}
			t.Errorf("Unexpected nsServer: %v", nsServer)
			return nil
		}

		ns, err := FindNSServers(context.Background(), "a.b.c.d")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(ns) != 2 {
			t.Errorf("Expected 2 NS servers, got %v", ns)
		}
		if ns[0].Host != "1.dns-servers.com" || ns[1].Host != "2.dns-servers.com" {
			t.Errorf("Wrong ns servers returned, got %v", ns)
		}
	})

	t.Run("NS server exist on domain (delegarted apex domain)", func(t *testing.T) {
		ResolverAt = func(nsServer string) Resolver {
			if strings.Contains(nsServer, "root-servers.net") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: []string{"1.tld-servers.com", "2.tld-servers.com"}, Error: nil},
				}}
			} else if strings.Contains(nsServer, "tld-servers.com") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: []string{"1.dns-servers.com", "2.dns-servers.com"}, Error: nil},
				}}
			} else if strings.Contains(nsServer, "dns-servers.com") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: []string{"1.delegated-servers.com", "2.delegated-servers.com"}, Error: nil},
				}}
			} else if strings.Contains(nsServer, "delegated-servers.com") {
				return MockResolver{Records: map[DNSRequest]DNSResponse{
					{Type: "NS", Domain: "a.b.c.d"}: {Records: []string{"2.delegated-servers.com", "1.delegated-servers.com"}, Error: nil},
				}}
			}
			t.Errorf("Unexpected nsServer: %v", nsServer)
			return nil
		}

		ns, err := FindNSServers(context.Background(), "a.b.c.d")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(ns) != 2 {
			t.Errorf("Expected 2 NS servers, got %v", ns)
		}
		if ns[0].Host != "1.delegated-servers.com" || ns[1].Host != "2.delegated-servers.com" {
			t.Errorf("Wrong ns servers returned, got %v", ns)
		}
	})
}

func TestRootResolver(t *testing.T) {
	t.Run("LookupIPAddr on google return both IPv4 and IPv6", func(t *testing.T) {
		r := RootResolver{}
		ips, err := r.LookupIPAddr(context.Background(), "www.google.com")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if len(ips) == 0 {
			t.Errorf("Expected some IP addresses, got %v", ips)
		}
		hasIPv4 := false
		hasIPv6 := false
		for _, ip := range ips {
			if ip.IP.To4() != nil {
				hasIPv4 = true
			} else if ip.IP.To16() != nil {
				hasIPv6 = true
			}
		}
		if !hasIPv4 || !hasIPv6 {
			t.Errorf("Expected both IPv4 and IPv6 addresses, got %v", ips)
		}
	})

	t.Run("LookupIPAddr on defang.io return same set of IPs", func(t *testing.T) {
		r := RootResolver{}
		ips, err := r.LookupIPAddr(context.Background(), "fabric-prod1.defang.dev")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		nIPs, err := net.LookupIP("fabric-prod1.defang.dev")
		if !SameIPs(IpAddrsToIPs(ips), nIPs) {
			t.Errorf("Expected same IP addresses, got %v <> %v", ips, nIPs)
		}
	})
}
