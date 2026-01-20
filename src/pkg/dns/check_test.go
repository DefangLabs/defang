package dns

import (
	"errors"
	"net"
	"slices"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/term"
)

var notFound = errors.New("not found")

func TestGetCNAMEInSync(t *testing.T) {
	t.Cleanup(func() {
		ResolverAt = DirectResolverAt
	})

	notFoundResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "web.test.com"}:    {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "CNAME", Domain: "web.test.com"}: {Records: nil, Error: notFound},
	}}
	foundResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "web.test.com"}:    {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "CNAME", Domain: "web.test.com"}: {Records: []string{"some-alb.domain.com"}, Error: nil},
	}}

	// Test when the domain is not found
	t.Run("domain not found", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return notFoundResolver }
		_, err := getCNAMEInSync(t.Context(), "web.test.com")
		if err != notFound {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	// Test when the domain is found but the DNS servers are not in sync
	t.Run("DNS servers not in sync", func(t *testing.T) {
		ResolverAt = func(nsServer string) Resolver {
			if nsServer == "ns1.example.com" {
				return foundResolver
			} else {
				return notFoundResolver
			}
		}
		_, err := getCNAMEInSync(t.Context(), "web.test.com")
		if err != errDNSNotInSync {
			t.Errorf("Expected NotInSync error, got %v", err)
		}
	})

	// Test when the domain is found and the DNS servers are in sync
	t.Run("DNS servers in sync", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return foundResolver }
		cname, err := getCNAMEInSync(t.Context(), "web.test.com")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if cname != "some-alb.domain.com" {
			t.Errorf("Returned IPs are not as expected")
		}
	})
}

func TestGetIPInSync(t *testing.T) {
	t.Cleanup(func() {
		ResolverAt = DirectResolverAt
	})

	notFoundResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "test.com"}: {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "test.com"}:  {Records: nil, Error: notFound},
	}}
	partialFoundResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "test.com"}: {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "test.com"}:  {Records: []string{"1.2.3.4"}, Error: nil},
	}}
	foundResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "test.com"}: {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "test.com"}:  {Records: []string{"1.2.3.4", "5.6.7.8"}, Error: nil},
	}}

	// Test when the domain is not found
	t.Run("domain not found", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return notFoundResolver }
		_, err := getIPInSync(t.Context(), "test.com")
		if err != notFound {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	// Test when the domain is found but the DNS servers are not in sync
	t.Run("DNS servers not in sync", func(t *testing.T) {
		ResolverAt = func(nsServer string) Resolver {
			if nsServer == "ns1.example.com" {
				return foundResolver
			} else {
				return notFoundResolver
			}
		}
		_, err := getIPInSync(t.Context(), "test.com")
		if err != errDNSNotInSync {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// 2nd not in sync scenario
	t.Run("DNS servers not in sync with partial results", func(t *testing.T) {
		ResolverAt = func(nsServer string) Resolver {
			if nsServer == "ns1.example.com" {
				return partialFoundResolver
			} else {
				return foundResolver
			}
		}
		_, err := getIPInSync(t.Context(), "test.com")
		if err != errDNSNotInSync {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// Test when the domain is found and the DNS servers are in sync
	t.Run("DNS servers in sync", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return foundResolver }
		ips, err := getIPInSync(t.Context(), "test.com")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !slices.EqualFunc([]string{"1.2.3.4", "5.6.7.8"}, ips, func(a string, b net.IP) bool { return a == b.String() }) {
			t.Errorf("Returned IPs are not as expected")
		}
	})
}

func TestCheckDomainDNSReady(t *testing.T) {
	emptyResolver := MockResolver{}
	hasARecordResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "api.test.com"}:       {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "some-alb.domain.com"}: {Records: []string{"1.2.3.4", "5,6,7,8"}, Error: nil},
		{Type: "A", Domain: "api.test.com"}:        {Records: []string{"1.2.3.4"}, Error: nil},
	}}
	hasWrongARecordResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "api.test.com"}:       {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "some-alb.domain.com"}: {Records: []string{"1.2.3.4", "5,6,7,8"}, Error: nil},
		{Type: "A", Domain: "api.test.com"}:        {Records: []string{"1.2.3.4", "9.9.9.9"}, Error: nil},
	}}
	hasCNAMEResolver := MockResolver{Records: map[DNSRequest]DNSResponse{
		{Type: "NS", Domain: "api.test.com"}:       {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "some-alb.domain.com"}: {Records: []string{"1.2.3.4", "5,6,7,8"}, Error: nil},
		{Type: "CNAME", Domain: "api.test.com"}:    {Records: []string{"some-alb.domain.com"}, Error: nil},
	}}
	resolver = hasARecordResolver

	oldResolver, oldDebug := ResolverAt, term.DoDebug()
	t.Cleanup(func() {
		ResolverAt = oldResolver
		term.SetDebug(oldDebug)
	})
	term.SetDebug(true)

	t.Run("CNAME and A records not found", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return emptyResolver }
		if CheckDomainDNSReady(t.Context(), "api.test.com", []string{"some-alb.domain.com"}) != false {
			t.Errorf("Expected false when both CNAME and A records are missing, got true")
		}
	})

	t.Run("CNAME setup correctly", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return hasCNAMEResolver }
		if CheckDomainDNSReady(t.Context(), "api.test.com", []string{"some-alb.domain.com"}) != true {
			t.Errorf("Expected true when CNAME is setup correctly, got false")
		}
	})

	t.Run("CNAME setup incorrectly", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return hasCNAMEResolver }
		if CheckDomainDNSReady(t.Context(), "api.test.com", []string{"some-other-alb.domain.com"}) != false {
			t.Errorf("Expected false when CNAME is setup incorrectly, got true")
		}
	})

	t.Run("A record setup correctly", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return hasARecordResolver }
		if CheckDomainDNSReady(t.Context(), "api.test.com", []string{"some-alb.domain.com"}) != true {
			t.Errorf("Expected true when A record is setup correctly, got false")
		}
	})
	t.Run("A record setup incorrectly", func(t *testing.T) {
		ResolverAt = func(_ string) Resolver { return hasWrongARecordResolver }
		if CheckDomainDNSReady(t.Context(), "api.test.com", []string{"some-alb.domain.com"}) != false {
			t.Errorf("Expected false when A record is setup incorrectly, got true")
		}
	})
}

func TestContainsAllIPs(t *testing.T) {
	a := net.ParseIP("1.1.1.1")
	b := net.ParseIP("1.1.1.2")
	c := net.ParseIP("1.1.1.3")
	d := net.ParseIP("1.1.1.4")

	tests := []struct {
		a, b []net.IP
		want bool
	}{
		{[]net.IP{}, []net.IP{}, true},
		{[]net.IP{a, b, c, d}, []net.IP{a, b}, true},
		{[]net.IP{d, c, b, a}, []net.IP{a, b}, true},
		{[]net.IP{b, a}, []net.IP{a, b}, true},
		{[]net.IP{a, b, c, d}, []net.IP{a, b, c, d}, true},
		{[]net.IP{a, c, d, b}, []net.IP{b, d, c, a}, true},
		{[]net.IP{a, b}, []net.IP{a, a, a, a}, true},
		{[]net.IP{a, b, b, b}, []net.IP{a, b, c, d}, false},
		{[]net.IP{}, []net.IP{a}, false},
	}

	for _, tt := range tests {
		if containsAllIPs(tt.a, tt.b) != tt.want {
			t.Errorf("%v contains all %v should be %v, but got %v", tt.a, tt.b, tt.want, !tt.want)
		}
	}
}

func TestSameIPs(t *testing.T) {
	a := net.ParseIP("1.1.1.1")
	b := net.ParseIP("1.1.1.2")
	c := net.ParseIP("1.1.1.3")
	d := net.ParseIP("1.1.1.4")

	tests := []struct {
		a, b []net.IP
		want bool
	}{
		{[]net.IP{}, []net.IP{}, true},
		{[]net.IP{a, b, c, d}, []net.IP{a, b, c, d}, true},
		{[]net.IP{d, c, b, a}, []net.IP{a, b, c, d}, true},
		{[]net.IP{b, a}, []net.IP{a, b}, true},
		{[]net.IP{a, b, b, d}, []net.IP{a, b, d, b}, true},
		{[]net.IP{a, a, a, b}, []net.IP{a, b, a, a}, true},
		{[]net.IP{a, b, b, b}, []net.IP{a, b, c, d}, false},
		{[]net.IP{a, b, b}, []net.IP{a, b, c}, false},
		{[]net.IP{a, b, b}, []net.IP{a, b}, false},
		{[]net.IP{a, b, b}, []net.IP{a, b, b, b}, false},
		{[]net.IP{a, b}, []net.IP{c, d}, false},
		{[]net.IP{}, []net.IP{a}, false},
		{[]net.IP{a}, []net.IP{b}, false},
		{[]net.IP{a}, []net.IP{}, false},
	}

	for _, tt := range tests {
		if SameIPs(tt.a, tt.b) != tt.want {
			t.Errorf("%v same IPs %v should be %v, but got %v", tt.a, tt.b, tt.want, !tt.want)
		}
	}
}
