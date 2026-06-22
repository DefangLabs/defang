package cert

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/dns"
)

// stallingListener accepts TCP connections and holds them open without
// speaking TLS or writing any bytes. This is how we exercise the per-attempt
// timeout: from the client's view, TCP succeeds but the TLS handshake never
// progresses.
func stallingListener(t *testing.T) (host string, port string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection until the test signals shutdown. Wrapping in
			// a goroutine so the accept loop is free to handle the (typical)
			// case of one connect attempt; the caller's per-attempt timeout
			// is what makes the test return.
			go func(c net.Conn) {
				<-stop
				c.Close()
			}(conn)
		}
	})

	t.Cleanup(func() {
		close(stop)
		ln.Close()
		wg.Wait()
	})

	host, port, err = net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	return host, port
}

// withTestPerAttemptTimeout temporarily shortens the per-attempt cap so the
// test doesn't sit on the 10s production default.
func withTestPerAttemptTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	prev := perAttemptTimeout
	perAttemptTimeout = d
	t.Cleanup(func() { perAttemptTimeout = prev })
}

func TestCheckTLSCert_PerAttemptTimeoutFiresOnStalledServer(t *testing.T) {
	withTestPerAttemptTimeout(t, 200*time.Millisecond)
	ip, port := stallingListener(t)

	// Build a domain that embeds the listener port so the URL becomes
	// https://<host>:<port>; getFixedIPTransport will dial <ip>:<port>.
	domain := "localhost:" + port
	resolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "A", Domain: domain}: {Records: []string{ip}},
	}}

	// Outer deadline is far larger than the per-attempt cap. If the cap
	// works the call returns in ~perAttemptTimeout; if it doesn't, this
	// test fails by context deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := CheckTLSCert(ctx, domain, resolver)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from stalled TLS handshake, got nil")
	}
	// 4x the cap is generous CI slack; the point is it's nowhere near the
	// outer 5s ctx deadline.
	if elapsed > 4*perAttemptTimeout {
		t.Fatalf("CheckTLSCert took %v, expected <= %v (per-attempt cap was %v)",
			elapsed, 4*perAttemptTimeout, perAttemptTimeout)
	}
	if ctx.Err() != nil {
		t.Fatalf("outer ctx expired (%v); per-attempt timeout did not fire first", ctx.Err())
	}
}

func TestCheckTLSCert_ResolverErrorPropagates(t *testing.T) {
	wantErr := &net.DNSError{Err: "no such host", Name: "missing.example"}
	resolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "A", Domain: "missing.example"}: {Error: wantErr},
	}}

	err := CheckTLSCert(context.Background(), "missing.example", resolver)
	if err == nil {
		t.Fatal("expected resolver error, got nil")
	}
	// CheckTLSCert wraps the resolver error with domain context so callers
	// (whose debug log might not otherwise mention the lookup stage) can tell
	// what failed.
	if !errors.Is(err, wantErr) {
		t.Errorf("wrapped error should still unwrap to the resolver error; got %v", err)
	}
	if !strings.Contains(err.Error(), "missing.example") {
		t.Errorf("error %q should name the domain", err)
	}
}

func TestCheckTLSCert_NoIPsReturnsError(t *testing.T) {
	// An empty A-record set is not TLS-ready. Every CheckTLSCert caller
	// (waitForTLS in pkg/cli/cert.go, and the Azure cert flow) treats nil
	// as "cert is online" and exits its polling loop — so returning nil
	// for zero probes would prematurely declare success. Surface an error
	// so callers keep waiting for DNS to populate.
	resolver := dns.MockResolver{Records: map[dns.DNSRequest]dns.DNSResponse{
		{Type: "A", Domain: "empty.example"}: {Records: nil},
	}}
	err := CheckTLSCert(context.Background(), "empty.example", resolver)
	if err == nil {
		t.Fatal("expected error for empty IP list, got nil")
	}
	if !strings.Contains(err.Error(), "empty.example") {
		t.Errorf("error %q should name the domain", err)
	}
}

func TestGetFixedIPTransport_HasResponseHeaderTimeout(t *testing.T) {
	// Regression guard: the response-header timeout is the second half of
	// the fix (the first being http.Client.Timeout in checkOne). If someone
	// later trims it back to zero, this fails so the fix isn't silently
	// undone.
	tr := getFixedIPTransport("127.0.0.1")
	if tr.ResponseHeaderTimeout <= 0 {
		t.Fatalf("ResponseHeaderTimeout = %v, want > 0", tr.ResponseHeaderTimeout)
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Fatalf("TLSHandshakeTimeout = %v, want > 0", tr.TLSHandshakeTimeout)
	}
}

// Ensure we wired the transport's DialContext to the fixed IP rather than
// honoring the URL host. Belt-and-suspenders test: if a refactor accidentally
// drops the IP pinning, this fails.
func TestGetFixedIPTransport_DialsFixedIP(t *testing.T) {
	tr := getFixedIPTransport("127.0.0.1")
	// We can't easily intercept the dial without rewriting the transport,
	// so just confirm the public knobs are present and DialContext is set.
	if tr.DialContext == nil {
		t.Fatal("DialContext is nil")
	}
	// A bogus host:port is fine — we only care that the dial target is
	// derived from the fixed IP, not the URL host. We assert by attempting
	// a dial against a closed port on 127.0.0.1 and expecting a connection
	// refused error rather than a name resolution error.
	_, err := tr.DialContext(context.Background(), "tcp", "doesnotexist.invalid:1")
	if err == nil {
		t.Fatal("expected dial error against closed port, got nil")
	}
	// On a fixed-IP transport the dial should never have done a DNS lookup
	// for "doesnotexist.invalid"; net.OpError wraps a "connection refused"
	// or similar. We don't pin to an exact string but check it's not a DNS
	// error message.
	if strings.Contains(err.Error(), "no such host") {
		t.Fatalf("dial appears to have resolved the URL host instead of using the fixed IP: %v", err)
	}
}

// Sanity check that http.Client is what we think — guards against a future
// refactor that drops Client.Timeout without realizing it's the user-visible
// cap that keeps waitForTLS's 3s ticker reachable.
func TestCheckOne_ClientHasTimeout(t *testing.T) {
	withTestPerAttemptTimeout(t, 50*time.Millisecond)
	ip, port := stallingListener(t)
	domain := "localhost:" + port

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	err := checkOne(ctx, domain, ip)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from stalled connection, got nil")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("checkOne took %v with %v cap; client timeout not honored",
			elapsed, perAttemptTimeout)
	}
	// Don't assert the error type — it can be either a context.DeadlineExceeded
	// (from attemptCtx) or url.Error wrapping it (from http.Client.Timeout),
	// depending on which fires first. Both are correct behavior.
}
