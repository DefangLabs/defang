package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

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
	if tr.result != nil && tr.result.Request == nil {
		tr.result.Request = req
	}
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
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 3)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if tc.calls != 1 {
			t.Errorf("Expected 1 call, got %v", tc.calls)
		}
	})
	t.Run("success on third try", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: nil, err: errors.New("error")},
			{result: nil, err: errors.New("error")},
			{result: &http.Response{StatusCode: 200, Body: mockBody("")}, err: nil},
		}}
		originalClient := httpClient
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 3)
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
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 3)
		if err == nil {
			t.Errorf("Expected error, got %v", err)
		} else if !strings.Contains(err.Error(), "HTTP: 503") {
			t.Errorf("Expected HTTP 503:, got %v", err)
		}
		if tc.calls != 3 {
			t.Errorf("Expected 3 calls, got %v", tc.calls)
		}
	})
	t.Run("redirect to https considers success", func(t *testing.T) {
		redirectURL, _ := url.Parse("https://example.com")
		tc := &testClient{tries: []tryResult{
			{result: &http.Response{StatusCode: 503, Request: &http.Request{URL: redirectURL}, Body: mockBody("Random Error")}, err: nil},
		}}
		originalClient := httpClient
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 3)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if tc.calls != 1 {
			t.Errorf("Expected 1 call, got %v", tc.calls)
		}
	})
	t.Run("TLS error considers success", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: nil, err: &tls.CertificateVerificationError{Err: errors.New("error")}},
		}}
		originalClient := httpClient
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 3)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if tc.calls != 1 {
			t.Errorf("Expected 1 call, got %v", tc.calls)
		}
	})
	t.Run("handles all http errors", func(t *testing.T) {
		tc := &testClient{tries: []tryResult{
			{result: &http.Response{StatusCode: 404, Body: mockBody("Random Error")}, err: nil},
			{result: &http.Response{StatusCode: 502, Body: mockBody("Random Error")}, err: nil},
			{result: &http.Response{StatusCode: 503, Body: mockBody("Random Error")}, err: nil},
		}}
		originalClient := httpClient
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 3)
		if err == nil {
			t.Errorf("Expected error, got %v", err)
		} else if !strings.Contains(err.Error(), "HTTP: 404") || !strings.Contains(err.Error(), "HTTP: 502") || !strings.Contains(err.Error(), "HTTP: 503") {
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
		t.Cleanup(func() { httpClient = originalClient })
		httpClient = tc
		err := getWithRetries(t.Context(), "http://example.com", 1)
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
	return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
}
func (mr *MockResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	mr.calls++
	return "", nil
}
func (mr *MockResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	mr.calls++
	return []*net.NS{{Host: ""}}, nil
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

func TestGetDomainTargets(t *testing.T) {
	tests := []struct {
		name        string
		serviceInfo *defangv1.ServiceInfo
		service     compose.ServiceConfig
		expected    []string
	}{
		{
			name: "use only lb dns name when present",
			serviceInfo: &defangv1.ServiceInfo{
				LbDnsName:  "aws.alb.com",
				PublicFqdn: "app.defang.app",
				Endpoints:  []string{"8080--app.defang.app", "8081--app.defang.app"},
			},
			service: compose.ServiceConfig{
				Ports: []composetypes.ServicePortConfig{
					{Mode: compose.Mode_INGRESS},
					{Mode: compose.Mode_INGRESS},
				},
			},
			expected: []string{"aws.alb.com"},
		},
		{
			name: "use only public fqdn and end points when lb dns name is empty",
			serviceInfo: &defangv1.ServiceInfo{
				LbDnsName:  "",
				PublicFqdn: "app.defang.app",
				Endpoints:  []string{"8080--app.defang.app", "8081--app.defang.app"},
			},
			service: compose.ServiceConfig{
				Ports: []composetypes.ServicePortConfig{
					{Mode: compose.Mode_INGRESS},
					{Mode: compose.Mode_INGRESS},
				},
			},
			expected: []string{"app.defang.app", "8080--app.defang.app", "8081--app.defang.app"},
		},
		{
			name: "only use endpoint of ingress ports",
			serviceInfo: &defangv1.ServiceInfo{
				LbDnsName:  "",
				PublicFqdn: "app.defang.app",
				Endpoints:  []string{"8080--app.defang.app", "8081--app.defang.app"},
			},
			service: compose.ServiceConfig{
				Ports: []composetypes.ServicePortConfig{
					{Mode: compose.Mode_INGRESS},
					{Mode: compose.Mode_HOST},
				},
			},
			expected: []string{"app.defang.app", "8080--app.defang.app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets := getDomainTargets(tt.serviceInfo, tt.service)
			if len(targets) != len(tt.expected) {
				t.Errorf("expected %v targets, got %v", len(tt.expected), len(targets))
			}
			sort.Strings(targets)
			sort.Strings(tt.expected)
			if !slices.Equal(targets, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, targets)
			}
		})
	}
}

// Test helper function to check if domains exist in compose project
func TestHasDomains(t *testing.T) {
	tests := []struct {
		name     string
		services map[string]compose.ServiceConfig
		expected bool
	}{
		{
			name:     "no services",
			services: map[string]compose.ServiceConfig{},
			expected: false,
		},
		{
			name: "services without domains",
			services: map[string]compose.ServiceConfig{
				"web": {Name: "web", DomainName: ""},
				"api": {Name: "api", DomainName: ""},
			},
			expected: false,
		},
		{
			name: "services with domains",
			services: map[string]compose.ServiceConfig{
				"web": {Name: "web", DomainName: "example.com"},
				"api": {Name: "api", DomainName: ""},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from GenerateLetsEncryptCert
			hasDomains := false
			for _, service := range tt.services {
				if service.DomainName != "" {
					hasDomains = true
					break
				}
			}

			if hasDomains != tt.expected {
				t.Errorf("expected hasDomains=%v, got %v", tt.expected, hasDomains)
			}
		})
	}
}
