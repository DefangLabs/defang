package dns

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"slices"
	"sort"

	"github.com/miekg/dns"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error)
	LookupCNAME(ctx context.Context, domain string) (string, error)
	LookupNS(ctx context.Context, domain string) ([]*net.NS, error)
}

type RootResolver struct{}

// https://en.wikipedia.org/wiki/Root_name_server
var rootServers = []*net.NS{
	{Host: "a.root-servers.net"},
	{Host: "b.root-servers.net"},
	{Host: "c.root-servers.net"},
	{Host: "d.root-servers.net"},
	{Host: "e.root-servers.net"},
	{Host: "f.root-servers.net"},
	{Host: "g.root-servers.net"},
	{Host: "h.root-servers.net"},
	{Host: "i.root-servers.net"},
	{Host: "j.root-servers.net"},
	{Host: "k.root-servers.net"},
	{Host: "l.root-servers.net"},
	{Host: "m.root-servers.net"},
}

func (r RootResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	return r.getResolver(ctx, domain).LookupIPAddr(ctx, domain)
}

func (r RootResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	return r.getResolver(ctx, domain).LookupCNAME(ctx, domain)
}

func (r RootResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	return r.getResolver(ctx, domain).LookupNS(ctx, domain)
}

func (r RootResolver) getResolver(ctx context.Context, domain string) Resolver {
	ns, err := FindNSServers(ctx, domain)
	if err != nil {
		return DirectResolver{}
	}
	return DirectResolver{NSServer: ns[rand.Intn(len(ns))].Host}
}

func FindNSServers(ctx context.Context, domain string) ([]*net.NS, error) {
	nsServers := rootServers
	retries := 3
	for {
		nsServer := nsServers[rand.Intn(len(nsServers))].Host
		ns, err := ResolverAt(nsServer).LookupNS(ctx, domain)
		sort.Slice(ns, func(i, j int) bool { return ns[i].Host < ns[j].Host })
		if err != nil {
			if retries--; retries > 0 {
				continue
			}
			return nil, err
		}
		if len(ns) == 0 || slices.EqualFunc(ns, nsServers, func(a, b *net.NS) bool { return a.Host == b.Host }) {
			return nsServers, nil
		}
		nsServers = ns
	}
}

var ResolverAt = func(nsServer string) Resolver {
	return DirectResolver{NSServer: nsServer}
}

var ErrNoSuchHost = &net.DNSError{Err: "no such host", IsNotFound: true}

type DirectResolver struct {
	NSServer string
}

func (r DirectResolver) query(ctx context.Context, domain string, qtype uint16) (*dns.Msg, error) {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), qtype)
	return dns.ExchangeContext(ctx, req, r.NSServer+":53")
}

func (r DirectResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	res, err := r.query(ctx, domain, dns.TypeNS)
	if err != nil {
		return nil, err
	}

	var result []net.IPAddr
	for _, rr := range res.Answer {
		if ns, ok := rr.(*dns.A); ok {
			result = append(result, net.IPAddr{IP: ns.A})
		} else if ns, ok := rr.(*dns.AAAA); ok {
			result = append(result, net.IPAddr{IP: ns.AAAA})
		}
	}
	if len(result) == 0 {
		return nil, ErrNoSuchHost
	}
	return result, nil
}

func (r DirectResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	res, err := r.query(ctx, domain, dns.TypeCNAME)
	if err != nil {
		return "", err
	}

	for _, rr := range res.Answer {
		fmt.Printf("%v -> %T\n", rr, rr)
		if ns, ok := rr.(*dns.CNAME); ok {
			return ns.Target, nil
		}
	}
	return "", ErrNoSuchHost
}

func (r DirectResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	res, err := r.query(ctx, domain, dns.TypeNS)
	if err != nil {
		return nil, err
	}

	var result []*net.NS
	for _, rr := range res.Ns {
		if ns, ok := rr.(*dns.NS); ok {
			result = append(result, &net.NS{Host: ns.Ns})
		}
	}
	return result, nil
}
