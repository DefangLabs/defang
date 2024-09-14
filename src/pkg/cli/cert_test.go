package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type dnsRequest struct {
	Type   string
	Domain string
}

var notFound = errors.New("not found")

func TestGetCNAMEInSync(t *testing.T) {
	t.Cleanup(func() {
		dns.ResolverAt = dns.DirectResolverAt
	})

	notFoundResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "web.test.com"}:    {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "CNAME", Domain: "web.test.com"}: {Records: nil, Error: notFound},
	}}
	foundResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "web.test.com"}:    {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "CNAME", Domain: "web.test.com"}: {Records: []string{"some-alb.domain.com"}, Error: nil},
	}}

	// Test when the domain is not found
	t.Run("domain not found", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return notFoundResolver }
		_, err := getCNAMEInSync(context.Background(), "web.test.com")
		if err != notFound {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	// Test when the domain is found but the DNS servers are not in sync
	t.Run("DNS servers not in sync", func(t *testing.T) {
		dns.ResolverAt = func(nsServer string) dns.Resolver {
			if nsServer == "ns1.example.com" {
				return foundResolver
			} else {
				return notFoundResolver
			}
		}
		_, err := getCNAMEInSync(context.Background(), "web.test.com")
		if err != errDNSNotInSync {
			t.Errorf("Expected NotInSync error, got %v", err)
		}
	})

	// Test when the domain is found and the DNS servers are in sync
	t.Run("DNS servers in sync", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return foundResolver }
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
	t.Cleanup(func() {
		dns.ResolverAt = dns.DirectResolverAt
	})

	notFoundResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "test.com"}: {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "test.com"}:  {Records: nil, Error: notFound},
	}}
	partialFoundResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "test.com"}: {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "test.com"}:  {Records: []string{"1.2.3.4"}, Error: nil},
	}}
	foundResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "test.com"}: {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "test.com"}:  {Records: []string{"1.2.3.4", "5.6.7.8"}, Error: nil},
	}}

	// Test when the domain is not found
	t.Run("domain not found", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return notFoundResolver }
		_, err := getIPInSync(context.Background(), "test.com")
		if err != notFound {
			t.Errorf("Expected NotFound error, got %v", err)
		}
	})

	// Test when the domain is found but the DNS servers are not in sync
	t.Run("DNS servers not in sync", func(t *testing.T) {
		dns.ResolverAt = func(nsServer string) dns.Resolver {
			if nsServer == "ns1.example.com" {
				return foundResolver
			} else {
				return notFoundResolver
			}
		}
		_, err := getIPInSync(context.Background(), "test.com")
		if err != errDNSNotInSync {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// 2nd not in sync scenario
	t.Run("DNS servers not in sync with partial results", func(t *testing.T) {
		dns.ResolverAt = func(nsServer string) dns.Resolver {
			if nsServer == "ns1.example.com" {
				return partialFoundResolver
			} else {
				return foundResolver
			}
		}
		_, err := getIPInSync(context.Background(), "test.com")
		if err != errDNSNotInSync {
			t.Errorf("Expected NotInSyncError error, got %v", err)
		}
	})

	// Test when the domain is found and the DNS servers are in sync
	t.Run("DNS servers in sync", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return foundResolver }
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
	term.SetDebug(true)
	emptyResolver := dns.MockResolver{}
	hasARecordResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "api.test.com"}:       {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "some-alb.domain.com"}: {Records: []string{"1.2.3.4", "5,6,7,8"}, Error: nil},
		{Type: "A", Domain: "api.test.com"}:        {Records: []string{"1.2.3.4"}, Error: nil},
	}}
	hasWrongARecordResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "api.test.com"}:       {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "some-alb.domain.com"}: {Records: []string{"1.2.3.4", "5,6,7,8"}, Error: nil},
		{Type: "A", Domain: "api.test.com"}:        {Records: []string{"1.2.3.4", "9.9.9.9"}, Error: nil},
	}}
	hasCNAMEResolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "NS", Domain: "api.test.com"}:       {Records: []string{"ns1.example.com", "ns2.example.com"}, Error: nil},
		{Type: "A", Domain: "some-alb.domain.com"}: {Records: []string{"1.2.3.4", "5,6,7,8"}, Error: nil},
		{Type: "CNAME", Domain: "api.test.com"}:    {Records: []string{"some-alb.domain.com"}, Error: nil},
	}}
	resolver = hasARecordResolver

	t.Cleanup(func() {
		dns.ResolverAt = dns.DirectResolverAt
	})

	t.Run("CNAME and A records not found", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return emptyResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", []string{"some-alb.domain.com"}) != false {
			t.Errorf("Expected false when both CNAME and A records are missing, got true")
		}
	})

	t.Run("CNAME setup correctly", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return hasCNAMEResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", []string{"some-alb.domain.com"}) != true {
			t.Errorf("Expected true when CNAME is setup correctly, got false")
		}
	})

	t.Run("CNAME setup incorrectly", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return hasCNAMEResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", []string{"some-other-alb.domain.com"}) != false {
			t.Errorf("Expected false when CNAME is setup incorrectly, got true")
		}
	})

	t.Run("A record setup correctly", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return hasARecordResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", []string{"some-alb.domain.com"}) != true {
			t.Errorf("Expected true when A record is setup correctly, got false")
		}
	})
	t.Run("A record setup incorrectly", func(t *testing.T) {
		dns.ResolverAt = func(_ string) dns.Resolver { return hasWrongARecordResolver }
		if checkDomainDNSReady(context.Background(), "api.test.com", []string{"some-alb.domain.com"}) != false {
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
		if dns.SameIPs(tt.a, tt.b) != tt.want {
			t.Errorf("%v same IPs %v should be %v, but got %v", tt.a, tt.b, tt.want, !tt.want)
		}
	}
}

type tryResult struct {
	result *http.Response
	err    error
}
type testClient struct {
	tries []tryResult
	calls int
}

var errNoMoreTries = errors.New("no more tries")

func (c *testClient) Do(req *http.Request) (*http.Response, error) {
	c.calls++
	if len(c.tries) == 0 {
		return nil, errNoMoreTries
	}
	tr := c.tries[0]
	c.tries = c.tries[1:]
	return tr.result, tr.err
}

func mockBody(s string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(s))
}

func TestGetWithRetries(t *testing.T) {
	originalDelayBase := httpRetryDelayBase
	httpRetryDelayBase = 100 * time.Millisecond
	t.Cleanup(func() { httpRetryDelayBase = originalDelayBase })

	t.Run("success on first try", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: &http.Response{StatusCode: 200, Body: mockBody("")}, err: nil},
		}}
		originalClient := httpClient
		defer func() { httpClient = originalClient }()
		httpClient = tc
		err := getWithRetries(context.Background(), "http://example.com", 3)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if tc.calls != 1 {
			t.Errorf("Expected 1 call, got %v", tc.calls)
		}
	})
	t.Run("success on thrid try", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: nil, err: errors.New("error")},
			{result: nil, err: errors.New("error")},
			{result: &http.Response{StatusCode: 200, Body: mockBody("")}, err: nil},
		}}
		originalClient := httpClient
		defer func() { httpClient = originalClient }()
		httpClient = tc
		err := getWithRetries(context.Background(), "http://example.com", 3)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if tc.calls != 3 {
			t.Errorf("Expected 3 calls, got %v", tc.calls)
		}
	})
	t.Run("handles http errors", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: nil, err: errors.New("error")},
			{result: &http.Response{StatusCode: 503, Body: mockBody("Random Error")}, err: nil},
			{result: nil, err: errors.New("error")},
		}}
		originalClient := httpClient
		defer func() { httpClient = originalClient }()
		httpClient = tc
		err := getWithRetries(context.Background(), "http://example.com", 3)
		if err == nil {
			t.Errorf("Expected error, got %v", err)
		} else if !strings.Contains(err.Error(), "HTTP 503: Random Error") {
			t.Errorf("Expected HTTP 503: Random Error, got %v", err)
		}
		if tc.calls != 3 {
			t.Errorf("Expected 3 calls, got %v", tc.calls)
		}
	})
	t.Run("handles all http errors", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: &http.Response{StatusCode: 404, Body: mockBody("Random Error")}, err: nil},
			{result: &http.Response{StatusCode: 502, Body: mockBody("Random Error")}, err: nil},
			{result: &http.Response{StatusCode: 503, Body: mockBody("Random Error")}, err: nil},
		}}
		originalClient := httpClient
		defer func() { httpClient = originalClient }()
		httpClient = tc
		err := getWithRetries(context.Background(), "http://example.com", 3)
		if err == nil {
			t.Errorf("Expected error, got %v", err)
		} else if !strings.Contains(err.Error(), "HTTP 404: Random Error") || !strings.Contains(err.Error(), "HTTP 502: Random Error") || !strings.Contains(err.Error(), "HTTP 503: Random Error") {
			t.Errorf("Expected HTTP 404,502,503 erros, got %v", err)
		}
		if tc.calls != 3 {
			t.Errorf("Expected 3 calls, got %v", tc.calls)
		}
	})
	t.Run("do not call more than requested", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: &http.Response{StatusCode: 404, Body: mockBody("Random Error")}, err: nil},
			{result: &http.Response{StatusCode: 502, Body: mockBody("Random Error")}, err: nil},
			{result: &http.Response{StatusCode: 503, Body: mockBody("Random Error")}, err: nil},
		}}
		originalClient := httpClient
		defer func() { httpClient = originalClient }()
		httpClient = tc
		err := getWithRetries(context.Background(), "http://example.com", 1)
		if err == nil {
			t.Errorf("Expected error, got %v", err)
		}
		if tc.calls != 1 {
			t.Errorf("Expected 3 calls, got %v", tc.calls)
		}
	})
}

type MockResolver struct {
	calls int
}

func (mr *MockResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	mr.calls++
	return []net.IPAddr{net.IPAddr{IP: net.ParseIP("127.0.0.1")}}, nil
}
func (mr *MockResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	mr.calls++
	return "", nil
}
func (mr *MockResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	mr.calls++
	return []*net.NS{&net.NS{Host: ""}}, nil
}

func TestHttpClient(t *testing.T) {

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()
	var mr MockResolver
	resolver = &mr
	dnsCacheDuration = 50 * time.Millisecond

	tsu, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("could not parse test server url '%v': %v", ts.URL, err)
	}
	_, port, err := net.SplitHostPort(tsu.Host)
	if err != nil {
		t.Fatalf("could not get test server port from '%v': %v", tsu.Host, err)
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("http://example.com:%v/", port), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make http call: %v", err)
	}
	resp.Body.Close()

	if mr.calls != 1 {
		t.Fatalf("expected 1 dns lookup, but got %v", mr.calls)
	}

	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make http call: %v", err)
	}
	resp.Body.Close()
	if mr.calls != 1 {
		t.Fatalf("expected no increase in dns lookup, but got %v", mr.calls)
	}

	time.Sleep(80 * time.Millisecond)
	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to make http call: %v", err)
	}
	resp.Body.Close()
	if mr.calls != 2 {
		t.Fatalf("expected 2nd dns lookup after cache expiry, but got %v", mr.calls)
	}

}
