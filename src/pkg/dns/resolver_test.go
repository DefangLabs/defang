package dns

import (
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

		ns, err := FindNSServers(t.Context(), "a.b.c.d")
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

		ns, err := FindNSServers(t.Context(), "a.b.c.d")
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
