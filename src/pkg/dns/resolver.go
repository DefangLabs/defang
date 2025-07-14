package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"

	"github.com/DefangLabs/defang/src/pkg"
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
	for range 10 {
		ips, err := r.getResolver(ctx, domain).LookupIPAddr(ctx, domain)
		if err != nil {
			if cnameErr, ok := err.(ErrCNAMEFound); ok {
				domain = cnameErr.CNAME()
				continue
			} else {
				return nil, err
			}
		}
		return ips, nil
	}
	return nil, errors.New("too many CNAME lookups")
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
	return DirectResolver{NSServer: ns[pkg.RandomIndex(len(ns))].Host}
}

func FindNSServers(ctx context.Context, domain string) ([]*net.NS, error) {
	nsServers := rootServers
	retries := 3
	for {
		index := pkg.RandomIndex(len(nsServers))
		nsServer := nsServers[index].Host
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

func DirectResolverAt(nsServer string) Resolver {
	return DirectResolver{NSServer: nsServer}
}

var ResolverAt = DirectResolverAt

var ErrNoSuchHost = &net.DNSError{Err: "no such host", IsNotFound: true}

type ErrCNAMEFound string

func (e ErrCNAMEFound) Error() string {
	return fmt.Sprintf("CNAME found: %v", string(e))
}

func (e ErrCNAMEFound) CNAME() string {
	return string(e)
}

type DirectResolver struct {
	NSServer string
}

func (r DirectResolver) query(ctx context.Context, domain string, qtype uint16) (*dns.Msg, error) {
	req := &dns.Msg{}
	req.SetQuestion(dns.Fqdn(domain), qtype)
	return dns.ExchangeContext(ctx, req, r.NSServer+":53")
}

func (r DirectResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	res, err := r.query(ctx, domain, dns.TypeA)
	if err != nil {
		return nil, err
	}

	var result []net.IPAddr
	var cname string
	var ansErr error
	for _, rr := range res.Answer {
		if ns, ok := rr.(*dns.A); ok {
			result = append(result, net.IPAddr{IP: ns.A})
		} else if cn, ok := rr.(*dns.CNAME); ok {
			cname = cn.Target
		} else {
			ansErr = fmt.Errorf("unexpected type %T [%v]", rr, rr)
		}
	}

	res, err = r.query(ctx, domain, dns.TypeAAAA)
	if err != nil {
		return nil, err
	}

	for _, rr := range res.Answer {
		if ns, ok := rr.(*dns.AAAA); ok {
			result = append(result, net.IPAddr{IP: ns.AAAA})
		} else if cn, ok := rr.(*dns.CNAME); ok {
			cname = cn.Target
		} else {
			ansErr = fmt.Errorf("unexpected type %T [%v]", rr, rr)
		}
	}
	if len(result) == 0 {
		if cname != "" {
			return nil, ErrCNAMEFound(cname)
		} else if ansErr != nil {
			return nil, ansErr
		}
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

func NSHosts(ns []*net.NS) []string {
	hosts := make([]string, len(ns))
	for i, n := range ns {
		hosts[i] = n.Host
	}
	return hosts
}

func SameIPs(a, b []net.IP) bool {
	if len(a) != len(b) {
		return false
	}
	diff := make(map[string]int)
	for _, ip := range a {
		diff[ip.String()]++
	}
	for _, ip := range b {
		diff[ip.String()]--
	}

	for _, v := range diff {
		if v != 0 {
			return false
		}
	}
	return true
}

func IpAddrsToIPs(ipAddrs []net.IPAddr) []net.IP {
	ips := make([]net.IP, len(ipAddrs))
	for i, ipAddr := range ipAddrs {
		ips[i] = ipAddr.IP
	}
	return ips
}
