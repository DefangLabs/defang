package cert

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/DefangLabs/defang/src/pkg/dns"
)

// perAttemptTimeout caps a single HTTPS probe against one IP so a server that
// completes TCP but stalls during TLS or the HTTP reply can't pin the caller
// for the full polling budget. waitForTLS retries on a 3s ticker, so this is
// the inner cap, not the total wait. Var rather than const so tests can
// shorten it.
var perAttemptTimeout = 10 * time.Second

func CheckTLSCert(ctx context.Context, domain string, resolver dns.Resolver) error {
	ips, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if err := checkOne(ctx, domain, ip.String()); err != nil {
			return err
		}
	}
	return nil
}

func checkOne(ctx context.Context, domain, ip string) error {
	attemptCtx, cancel := context.WithTimeout(ctx, perAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, "https://"+domain, nil)
	if err != nil {
		return err
	}
	httpClient := &http.Client{
		Transport: getFixedIPTransport(ip),
		Timeout:   perAttemptTimeout,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused; cap the read so a chatty server
	// can't extend the attempt past its deadline.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<14))
	return nil
}

func getFixedIPTransport(ip string) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			dialer := &net.Dialer{}
			rootAddr := net.JoinHostPort(ip, port)
			return dialer.DialContext(ctx, network, rootAddr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
