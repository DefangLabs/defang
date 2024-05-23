package dns

import (
	"context"
	"net"
)

type DNSRequest struct {
	Type   string
	Domain string
}

type DNSResponse struct {
	Records []string
	Error   error
}

type MockResolver struct {
	Records map[DNSRequest]DNSResponse
}

type ErrUnexpectedRequest DNSRequest

func (e ErrUnexpectedRequest) Error() string {
	return "Unexpected request: " + DNSRequest(e).Domain + " " + DNSRequest(e).Type
}

func (r MockResolver) records(req DNSRequest) ([]string, error) {
	res, ok := r.Records[req]
	if !ok {
		return nil, ErrUnexpectedRequest(req)
	}
	return res.Records, res.Error
}

func convert[E any](a []string, f func(string) E) []E {
	b := make([]E, len(a))
	for i, v := range a {
		b[i] = f(v)
	}
	return b
}

func (r MockResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	ips, err := r.records(DNSRequest{Type: "A", Domain: domain})
	return convert(ips, func(ip string) net.IPAddr { return net.IPAddr{IP: net.ParseIP(ip)} }), err
}
func (r MockResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	cnames, err := r.records(DNSRequest{Type: "CNAME", Domain: domain})
	if err != nil {
		return "", err
	}
	return cnames[0], nil
}
func (r MockResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	ns, err := r.records(DNSRequest{Type: "NS", Domain: domain})
	return convert(ns, func(n string) *net.NS { return &net.NS{Host: n} }), err
}
