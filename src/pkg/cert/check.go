package cert

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/DefangLabs/defang/src/pkg/dns"
)

func CheckTLSCert(ctx context.Context, domain string) error {
	resolver := dns.RootResolver{}
	ips, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		url := "https://" + domain
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		httpClient := &http.Client{
			Transport: getFixedIPTransport(ip.String()),
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	}
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
		ExpectContinueTimeout: 1 * time.Second,
	}
}
