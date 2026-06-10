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
	"sync"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/term"
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
		err := getWithRetries(t.Context(), "http://example.com", 3, tc)
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
		err := getWithRetries(t.Context(), "http://example.com", 3, tc)
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
		err := getWithRetries(t.Context(), "http://example.com", 3, tc)
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
		err := getWithRetries(t.Context(), "http://example.com", 3, tc)
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
		err := getWithRetries(t.Context(), "http://example.com", 3, tc)
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
		err := getWithRetries(t.Context(), "http://example.com", 3, tc)
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
		err := getWithRetries(t.Context(), "http://example.com", 1, tc)
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
func (mr *MockResolver) LookupTXT(ctx context.Context, domain string) ([]string, error) {
	mr.calls++
	return nil, nil
}

func TestHttpClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello, client")
	}))
	defer ts.Close()
	var mr MockResolver
	dnsCacheDuration = 50 * time.Millisecond

	tc := newCertHTTPClient(&mr)

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

	resp, err := tc.Do(req)
	if err != nil {
		t.Fatalf("failed to make http call: %v", err)
	}
	resp.Body.Close()

	if mr.calls != 1 {
		t.Fatalf("expected 1 dns lookup, but got %v", mr.calls)
	}

	resp, err = tc.Do(req)
	if err != nil {
		t.Fatalf("failed to make http call: %v", err)
	}
	resp.Body.Close()
	if mr.calls != 1 {
		t.Fatalf("expected no increase in dns lookup, but got %v", mr.calls)
	}

	time.Sleep(80 * time.Millisecond)
	resp, err = tc.Do(req)
	if err != nil {
		t.Fatalf("failed to make http call: %v", err)
	}
	resp.Body.Close()
	if mr.calls != 2 {
		t.Fatalf("expected 2nd dns lookup after cache expiry, but got %v", mr.calls)
	}
}

type mockCertProvider struct {
	client.MockProvider
	services *defangv1.GetServicesResponse
	err      error
}

func (m *mockCertProvider) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return m.services, m.err
}

type mockCertFabricClient struct {
	client.MockFabricClient
	mu             sync.Mutex
	verifyDNSCalls int
}

func (m *mockCertFabricClient) VerifyDNSSetup(ctx context.Context, req *defangv1.VerifyDNSSetupRequest) error {
	m.mu.Lock()
	m.verifyDNSCalls++
	m.mu.Unlock()
	return nil // DNS verified successfully
}

func (m *mockCertFabricClient) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.verifyDNSCalls
}

// mockCertIssuerProvider extends mockCertProvider with the CertIssuer
// interface so GenerateLetsEncryptCert's `provider.(CertIssuer)` succeeds.
// Captures every (project, service, hostname) tuple the SUT calls IssueCert
// with, and lets the test inject an error per call. The mutex makes the call
// log safe under the new parallel-worker dispatch.
type mockCertIssuerProvider struct {
	mockCertProvider
	mu        sync.Mutex
	issueErr  error
	issueCall []string
}

func (m *mockCertIssuerProvider) IssueCert(_ context.Context, projectName, serviceName, hostname string, _ func(string) dns.Resolver) error {
	m.mu.Lock()
	m.issueCall = append(m.issueCall, fmt.Sprintf("%s/%s/%s", projectName, serviceName, hostname))
	m.mu.Unlock()
	return m.issueErr
}

func (m *mockCertIssuerProvider) sortedCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := slices.Clone(m.issueCall)
	sort.Strings(out)
	return out
}

func TestGenerateLetsEncryptCert(t *testing.T) {
	t.Run("error when no services", func(t *testing.T) {
		provider := &mockCertProvider{
			services: &defangv1.GetServicesResponse{Services: nil},
		}
		project := &compose.Project{Name: "test"}
		err := GenerateLetsEncryptCert(t.Context(), project, nil, provider)
		if err == nil {
			t.Fatal("expected error for empty services")
		}
		if !strings.Contains(err.Error(), "no services found") {
			t.Errorf("expected 'no services found' error, got: %v", err)
		}
	})

	t.Run("GetServices error propagated", func(t *testing.T) {
		provider := &mockCertProvider{
			err: errors.New("provider error"),
		}
		project := &compose.Project{Name: "test"}
		err := GenerateLetsEncryptCert(t.Context(), project, nil, provider)
		if err == nil || err.Error() != "provider error" {
			t.Errorf("expected provider error, got: %v", err)
		}
	})

	t.Run("skips service without UseAcmeCert", func(t *testing.T) {
		provider := &mockCertProvider{
			services: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{
						Service:     &defangv1.Service{Name: "web"},
						UseAcmeCert: false,
						Domainname:  "example.com",
					},
				},
			},
		}
		project := &compose.Project{
			Name: "test",
			Services: compose.Services{
				"web": {DomainName: "example.com"},
			},
		}
		// Should not error and should log "no domainname found" (cnt == 0)
		err := GenerateLetsEncryptCert(t.Context(), project, nil, provider)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("skips service not in project", func(t *testing.T) {
		provider := &mockCertProvider{
			services: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{
						Service:     &defangv1.Service{Name: "unknown"},
						UseAcmeCert: true,
						Domainname:  "example.com",
					},
				},
			},
		}
		project := &compose.Project{
			Name:     "test",
			Services: compose.Services{},
		}
		err := GenerateLetsEncryptCert(t.Context(), project, nil, provider)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("calls ACME workflow for UseAcmeCert service", func(t *testing.T) {
		fabricClient := &mockCertFabricClient{}
		provider := &mockCertProvider{
			services: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{
						Service:     &defangv1.Service{Name: "web"},
						UseAcmeCert: true,
						Domainname:  "example.com",
						PublicFqdn:  "web.test.defang.app",
						Endpoints:   []string{"8080--web.test.defang.app"},
					},
				},
			},
		}
		project := &compose.Project{
			Name: "test",
			Services: compose.Services{
				"web": {
					Name:       "web",
					DomainName: "example.com",
					Ports: []composetypes.ServicePortConfig{
						{Mode: compose.Mode_INGRESS},
					},
				},
			},
		}
		// Short ctx ensures we don't block on the real 10-minute waitForTLS.
		// With preflight returning verified, the worker proceeds to the post-
		// verification stages and the deadline trips during the propagation
		// sleep — so the error is expected. The point of the test is that
		// the preflight RPC actually ran (i.e. we reached the ACME path).
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		err := GenerateLetsEncryptCert(ctx, project, fabricClient, provider)
		if err == nil {
			t.Error("expected deadline-related error from the short ctx, got nil")
		} else if !strings.Contains(err.Error(), "example.com") {
			t.Errorf("error should name the failing domain, got: %v", err)
		}
		if fabricClient.calls() == 0 {
			t.Error("expected VerifyDNSSetup to be called (ACME path was reached)")
		}
	})

	// CertIssuer dispatch: when the provider implements CertIssuer, the legacy
	// fabric/ACME path (VerifyDNSSetup + generateCert) is bypassed entirely and
	// IssueCert is invoked once per domain (the service domainname plus any
	// default-network aliases).
	t.Run("dispatches to CertIssuer when provider implements it", func(t *testing.T) {
		fabricClient := &mockCertFabricClient{}
		provider := &mockCertIssuerProvider{
			mockCertProvider: mockCertProvider{
				services: &defangv1.GetServicesResponse{
					Services: []*defangv1.ServiceInfo{
						{
							Service:     &defangv1.Service{Name: "web"},
							UseAcmeCert: true,
							Domainname:  "example.com",
						},
					},
				},
			},
		}
		project := &compose.Project{
			Name: "myproj",
			Services: compose.Services{
				"web": {
					Name:       "web",
					DomainName: "example.com",
					Networks: map[string]*composetypes.ServiceNetworkConfig{
						"default": {Aliases: []string{"alias.example.com"}},
					},
				},
			},
		}
		err := GenerateLetsEncryptCert(t.Context(), project, fabricClient, provider)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// Order-insensitive: workers run in parallel so the calls can land in
		// either order. Set membership is what we care about.
		want := []string{"myproj/web/alias.example.com", "myproj/web/example.com"}
		if got := provider.sortedCalls(); !slices.Equal(got, want) {
			t.Errorf("IssueCert calls = %v, want %v (any order)", got, want)
		}
		// Issuer path is short-circuited — the ACME codepath (which calls
		// VerifyDNSSetup) must not run.
		if fabricClient.calls() != 0 {
			t.Errorf("VerifyDNSSetup should not be called when CertIssuer is used (got %d calls)", fabricClient.calls())
		}
	})

	// IssueCert errors must surface — they used to be silently swallowed.
	// Each failure is collected and joined into the returned error so the
	// CLI exit code reflects the issuance outcome.
	t.Run("aggregates IssueCert errors", func(t *testing.T) {
		provider := &mockCertIssuerProvider{
			mockCertProvider: mockCertProvider{
				services: &defangv1.GetServicesResponse{
					Services: []*defangv1.ServiceInfo{
						{
							Service:     &defangv1.Service{Name: "web"},
							UseAcmeCert: true,
							Domainname:  "example.com",
						},
					},
				},
			},
			issueErr: errors.New("dns timeout"),
		}
		project := &compose.Project{
			Name: "myproj",
			Services: compose.Services{
				"web": {Name: "web", DomainName: "example.com"},
			},
		}
		err := GenerateLetsEncryptCert(t.Context(), project, &mockCertFabricClient{}, provider)
		if err == nil {
			t.Fatal("expected error when IssueCert fails")
		}
		if !strings.Contains(err.Error(), "dns timeout") {
			t.Errorf("error %q should mention the underlying IssueCert error", err)
		}
		if !strings.Contains(err.Error(), "example.com") {
			t.Errorf("error %q should name the failing domain", err)
		}
	})
}

// preflightFabricClient lets a test program responses per-domain so we can
// drive the preflight-bucketing logic without touching the network.
type preflightFabricClient struct {
	client.MockFabricClient
	mu        sync.Mutex
	responses map[string]error // domain -> err returned from VerifyDNSSetup
	calls     []string         // captured domains, in call order
}

func (m *preflightFabricClient) VerifyDNSSetup(_ context.Context, req *defangv1.VerifyDNSSetupRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, req.Domain)
	if err, ok := m.responses[req.Domain]; ok {
		return err
	}
	return nil
}

func TestPreflightDNS(t *testing.T) {
	jobs := []domainJob{
		{serviceName: "web", domain: "ok.example.com", targets: []string{"def-abc"}},
		{serviceName: "web", domain: "bad.example.com", targets: []string{"def-abc"}},
		{serviceName: "api", domain: "also-ok.example.com", targets: []string{"def-xyz"}},
	}
	fab := &preflightFabricClient{responses: map[string]error{
		"bad.example.com": errors.New("not ready"),
	}}

	verified := preflightDNS(t.Context(), jobs, fab)

	if !verified["ok.example.com"] || !verified["also-ok.example.com"] {
		t.Errorf("expected ok and also-ok to be verified, got %v", verified)
	}
	if verified["bad.example.com"] {
		t.Errorf("expected bad to not be verified, got %v", verified)
	}
	if got, want := len(fab.calls), len(jobs); got != want {
		t.Errorf("VerifyDNSSetup calls = %d, want %d (once per job)", got, want)
	}
}

func TestCollectDomainJobs(t *testing.T) {
	tests := []struct {
		name     string
		services []*defangv1.ServiceInfo
		project  *compose.Project
		want     []string // domains, in order
	}{
		{
			name: "skips UseAcmeCert=false",
			services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "web"}, UseAcmeCert: false, Domainname: "skip.example.com"},
			},
			project: &compose.Project{Services: compose.Services{
				"web": {Name: "web", DomainName: "skip.example.com"},
			}},
			want: nil,
		},
		{
			name: "skips service not in project",
			services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "ghost"}, UseAcmeCert: true, Domainname: "ghost.example.com"},
			},
			project: &compose.Project{Services: compose.Services{}},
			want:    nil,
		},
		{
			name: "skips when deployed Domainname is empty",
			services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "web"}, UseAcmeCert: true, Domainname: ""},
			},
			project: &compose.Project{Services: compose.Services{
				"web": {Name: "web", DomainName: "wishful.example.com"},
			}},
			want: nil,
		},
		{
			name: "flattens primary domain plus default-network aliases",
			services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "web"}, UseAcmeCert: true, Domainname: "web.example.com"},
			},
			project: &compose.Project{Services: compose.Services{
				"web": {Name: "web", DomainName: "web.example.com", Networks: map[string]*composetypes.ServiceNetworkConfig{
					"default": {Aliases: []string{"alt.example.com", "third.example.com"}},
				}},
			}},
			want: []string{"web.example.com", "alt.example.com", "third.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := collectDomainJobs(tt.project, tt.services)
			got := make([]string, len(jobs))
			for i, j := range jobs {
				got[i] = j.domain
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("domains = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewDomainLogger_PrefixAlignment(t *testing.T) {
	stdout, _ := term.SetupTestTerm(t)

	// Pad to the longest domain so columns line up across workers.
	pad := len("longest.example.com")
	short := newDomainLogger("a.example.com", pad)
	long := newDomainLogger("longest.example.com", pad)
	short("hello")
	long("hello")

	got := stripANSI(stdout.String())
	// Each line should start with a bracketed domain followed by enough
	// spaces that the message column lines up.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 output lines, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], " * [a.example.com]") {
		t.Errorf("short line missing bracket prefix: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], " * [longest.example.com]") {
		t.Errorf("long line missing bracket prefix: %q", lines[1])
	}
	// Find the "hello" column on both lines — they must match for alignment.
	col0 := strings.Index(lines[0], "hello")
	col1 := strings.Index(lines[1], "hello")
	if col0 != col1 {
		t.Errorf("message column not aligned: %d vs %d (%q / %q)", col0, col1, lines[0], lines[1])
	}
}

// TestRunIssuerJobs_ParallelMultipleDomains exercises the parallel issuer
// path: with N>1 jobs, IssueCert must be called for every job (set-equality)
// and concurrency cap must not deadlock.
func TestRunIssuerJobs_ParallelMultipleDomains(t *testing.T) {
	provider := &mockCertIssuerProvider{
		mockCertProvider: mockCertProvider{
			services: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{Service: &defangv1.Service{Name: "web"}, UseAcmeCert: true, Domainname: "a.example.com"},
					{Service: &defangv1.Service{Name: "api"}, UseAcmeCert: true, Domainname: "b.example.com"},
					{Service: &defangv1.Service{Name: "admin"}, UseAcmeCert: true, Domainname: "c.example.com"},
				},
			},
		},
	}
	project := &compose.Project{
		Name: "multi",
		Services: compose.Services{
			"web":   {Name: "web", DomainName: "a.example.com"},
			"api":   {Name: "api", DomainName: "b.example.com"},
			"admin": {Name: "admin", DomainName: "c.example.com"},
		},
	}
	if err := GenerateLetsEncryptCert(t.Context(), project, &mockCertFabricClient{}, provider); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{
		"multi/admin/c.example.com",
		"multi/api/b.example.com",
		"multi/web/a.example.com",
	}
	if got := provider.sortedCalls(); !slices.Equal(got, want) {
		t.Errorf("issued = %v, want %v (any order)", got, want)
	}
}

// stripANSI removes the colour escape sequences that term.Infof prepends so
// tests can assert on the post-formatting plain text. The escapes are simple
// CSI sequences (ESC `[` … `m`) which is all we generate.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
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
