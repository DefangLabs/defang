package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"sort"

	"github.com/DefangLabs/defang/src/pkg"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/miekg/dns"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error)
	LookupCNAME(ctx context.Context, domain string) (string, error)
	LookupNS(ctx context.Context, domain string) ([]*net.NS, error)
}

// FabricResolverClient is the subset of the fabric gRPC API used to resolve DNS
// records remotely.
type FabricResolverClient interface {
	ResolveIPAddr(context.Context, *defangv1.ResolveIPAddrRequest) (*defangv1.ResolveIPAddrResponse, error)
	ResolveCNAME(context.Context, *defangv1.ResolveCNAMERequest) (*defangv1.ResolveCNAMEResponse, error)
	ResolveNS(context.Context, *defangv1.ResolveNSRequest) (*defangv1.ResolveNSResponse, error)
}

// FabricResolver performs DNS lookups via the fabric gRPC API. An empty
// NSServer lets the server perform recursive resolution from the root.
type FabricResolver struct {
	Client   FabricResolverClient
	NSServer string
}

func (r FabricResolver) LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error) {
	resp, err := r.Client.ResolveIPAddr(ctx, &defangv1.ResolveIPAddrRequest{
		Domain:   domain,
		NsServer: r.NSServer,
	})
	if err != nil {
		return nil, err
	}
	ips := make([]net.IPAddr, 0, len(resp.IpAddrs))
	for _, s := range resp.IpAddrs {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, net.IPAddr{IP: ip})
		}
	}
	if len(ips) == 0 {
		return nil, ErrNoSuchHost
	}
	return ips, nil
}

func (r FabricResolver) LookupCNAME(ctx context.Context, domain string) (string, error) {
	resp, err := r.Client.ResolveCNAME(ctx, &defangv1.ResolveCNAMERequest{
		Domain:   domain,
		NsServer: r.NSServer,
	})
	if err != nil {
		return "", err
	}
	if resp.Cname == "" {
		return "", ErrNoSuchHost
	}
	return resp.Cname, nil
}

func (r FabricResolver) LookupNS(ctx context.Context, domain string) ([]*net.NS, error) {
	resp, err := r.Client.ResolveNS(ctx, &defangv1.ResolveNSRequest{
		Domain:   domain,
		NsServer: r.NSServer,
	})
	if err != nil {
		return nil, err
	}
	nss := make([]*net.NS, 0, len(resp.Hosts))
	for _, h := range resp.Hosts {
		nss = append(nss, &net.NS{Host: h})
	}
	return nss, nil
}

// RootResolver performs recursive DNS resolution starting from the root
// nameservers. Set ResolverAt to override how individual nameservers are
// queried (e.g. to route through the Fabric gRPC API). A nil ResolverAt
// falls back to DirectResolverAt.
type RootResolver struct {
	ResolverAt func(string) Resolver
}

func (r RootResolver) resolverFn() func(string) Resolver {
	if r.ResolverAt != nil {
		return r.ResolverAt
	}
	return DirectResolverAt
}

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
	ns, err := FindNSServers(ctx, domain, r.resolverFn())
	if err != nil {
		return DirectResolver{}
	}
	return DirectResolver{NSServer: ns[pkg.RandomIndex(len(ns))].Host}
}

func FindNSServers(ctx context.Context, domain string, resolverAt func(string) Resolver) ([]*net.NS, error) {
	nsServers := rootServers
	retries := 3
	for {
		index := pkg.RandomIndex(len(nsServers))
		nsServer := nsServers[index].Host
		ns, err := resolverAt(nsServer).LookupNS(ctx, domain)
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
	if nsServer == "" {
		return RootResolver{}
	}
	return DirectResolver{NSServer: nsServer}
}

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
	// When the name server is not the authoritative server for the domain,
	// the authoritative NS records are in the authority section.
	for _, rr := range res.Ns {
		if ns, ok := rr.(*dns.NS); ok {
			result = append(result, &net.NS{Host: ns.Ns})
		}
	}
	// When the name server is authoritative for the domain,
	// the NS records are in the answer section.
	for _, rr := range res.Answer {
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
