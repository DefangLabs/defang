package cli

import (
	"context"
	"net"
	"slices"
	"testing"
)

type dnsRequest struct {
	Type   string
	Domain string
}

type mockResolver struct {
	Records map[dnsRequest][]string
}

var NotFound = &net.DNSError{Err: "no such host"}

func (r mockResolver) records(req dnsRequest) ([]string, error) {
	records, ok := r.Records[req]
	if !ok || len(records) == 0 {
		return nil, NotFound
	}
	return records, nil
}

func (r mockResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	ips, err := r.records(dnsRequest{Type: "A", Domain: domain})
	if err != nil {
		return nil, err
	}
	var ipAddrs []net.IPAddr
	for _, ip := range ips {
		ipAddrs = append(ipAddrs, net.IPAddr{IP: net.ParseIP(ip)})
	}
	return ipAddrs, nil
}
func (r mockResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	cnames, err := r.records(dnsRequest{Type: "CNAME", Domain: domain})
	if err != nil {
		return "", err
	}
	return cnames[0], nil
}
func (r mockResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	ns, err := r.records(dnsRequest{Type: "NS", Domain: domain})
	if err != nil {
		return nil, err
	}
	var nsRecords []*net.NS
	for _, n := range ns {
		nsRecords = append(nsRecords, &net.NS{Host: n})
	}
	return nsRecords, nil
}

func TestGetCNAMEInSync(t *testing.T) {
	nsResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "NS", Domain: "test.com"}: {"ns1.example.com", "ns2.example.com"},
	}}
	notFoundResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "CNAME", Domain: "web.test.com"}: nil,
	}}
	foundResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "CNAME", Domain: "web.test.com"}: {"some-alb.domain.com"},
	}}
	resolver = nsResolver

	// Test when the domain is not found
	t.Run("domain not found", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return notFoundResolver }
		_, err := getCNAMEInSync(context.Background(), "web.test.com")
		if err != NotFound {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	// Test when the domain is found but the DNS servers are not in sync
	t.Run("DNS servers not in sync", func(t *testing.T) {
		resolverAt = func(nsServer string) Resolver {
			if nsServer == "ns1.example.com" {
				return foundResolver
			} else {
				return notFoundResolver
			}
		}
		_, err := getCNAMEInSync(context.Background(), "web.test.com")
		if err != DNSNotInSyncError {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// Test when the domain is found and the DNS servers are in sync
	t.Run("DNS servers in sync", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return foundResolver }
		cname, err := getCNAMEInSync(context.Background(), "web.test.com")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if cname != "some-alb.domain.com" {
			t.Errorf("Returned IPs are not as expected")
		}
	})
}

func TestGetIPInSync(t *testing.T) {
	nsResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "NS", Domain: "test.com"}: {"ns1.example.com", "ns2.example.com"},
	}}
	notFoundResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "A", Domain: "test.com"}: nil,
	}}
	partialFoundResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "A", Domain: "test.com"}: {"1.2.3.4"},
	}}
	foundResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "A", Domain: "test.com"}: {"1.2.3.4", "5.6.7.8"},
	}}
	resolver = nsResolver

	// Test when the domain is not found
	t.Run("domain not found", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return notFoundResolver }
		_, err := getIPInSync(context.Background(), "test.com")
		if err != NotFound {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	// Test when the domain is found but the DNS servers are not in sync
	t.Run("DNS servers not in sync", func(t *testing.T) {
		resolverAt = func(nsServer string) Resolver {
			if nsServer == "ns1.example.com" {
				return foundResolver
			} else {
				return notFoundResolver
			}
		}
		_, err := getIPInSync(context.Background(), "test.com")
		if err != DNSNotInSyncError {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// 2nd not in sync scenario
	t.Run("DNS servers not in sync with partial results", func(t *testing.T) {
		resolverAt = func(nsServer string) Resolver {
			if nsServer == "ns1.example.com" {
				return partialFoundResolver
			} else {
				return foundResolver
			}
		}
		_, err := getIPInSync(context.Background(), "test.com")
		if err != DNSNotInSyncError {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// Test when the domain is found and the DNS servers are in sync
	t.Run("DNS servers in sync", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return foundResolver }
		ips, err := getIPInSync(context.Background(), "test.com")
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if !slices.EqualFunc([]string{"1.2.3.4", "5.6.7.8"}, ips, func(a string, b net.IP) bool { return a == b.String() }) {
			t.Errorf("Returned IPs are not as expected")
		}
	})
}

func TestCheckDomainDNSReady(t *testing.T) {
	nsResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "NS", Domain: "test.com"}:           {"ns1.example.com", "ns2.example.com"},
		{Type: "A", Domain: "some-alb.domain.com"}: {"1.2.3.4", "5,6,7,8"},
	}}
	emptyResolver := mockResolver{}
	hasARecordResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "A", Domain: "api.test.com"}: {"1.2.3.4"},
	}}
	hasWrongARecordResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "A", Domain: "api.test.com"}: {"1.2.3.4", "9.9.9.9"},
	}}
	hasCNAMEResolver := mockResolver{Records: map[dnsRequest][]string{
		{Type: "CNAME", Domain: "api.test.com"}: {"some-alb.domain.com"},
	}}
	resolver = nsResolver

	t.Run("CNAME and A records not found", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return emptyResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", "some-alb.domain.com") != false {
			t.Errorf("Expected false when both CNAME and A records are missing, got true")
		}
	})

	t.Run("CNAME setup correctly", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return hasCNAMEResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", "some-alb.domain.com") != true {
			t.Errorf("Expected true when CNAME is setup correctly, got false")
		}
	})

	t.Run("CNAME setup incorrectly", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return hasCNAMEResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", "some-other-alb.domain.com") != false {
			t.Errorf("Expected false when CNAME is setup incorrectly, got true")
		}
	})

	t.Run("A record setup correctly", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return hasARecordResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", "some-alb.domain.com") != true {
			t.Errorf("Expected true when A record is setup correctly, got false")
		}
	})
	t.Run("A record setup incorrectly", func(t *testing.T) {
		resolverAt = func(_ string) Resolver { return hasWrongARecordResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", "some-alb.domain.com") != false {
			t.Errorf("Expected false when A record is setup incorrectly, got true")
		}
	})
}
