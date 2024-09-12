//go:build integration

package dns

import (
	"context"
	"net"
	"testing"
)

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

	t.Run("LookupIPAddr follows multiple CNAME redirects", func(t *testing.T) {
		r := RootResolver{}
		ips, err := r.LookupIPAddr(context.Background(), "cname-a.gnafed.click")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !SameIPs(IpAddrsToIPs(ips), []net.IP{net.ParseIP("1.2.3.4")}) {
			t.Errorf("Expected IP address 1.2.3.4 got %v", ips)
		}
	})
}
