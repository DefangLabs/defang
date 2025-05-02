package dns

import (
	"context"
	"errors"
	"net"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type logger interface {
	Debugf(format string, args ...any) (int, error)
	Infof(format string, args ...any) (int, error)
	Warnf(format string, args ...any) (int, error)
	Errorf(format string, args ...any) (int, error)
}

var (
	Logger   logger   = term.DefaultTerm
	resolver Resolver = RootResolver{}

	errDNSNotInSync = errors.New("DNS not in sync")
)

// The DNS is considered ready if the CNAME of the domain is pointing to the ALB domain and in sync
// OR if the A record of the domain is pointing to the same IP addresses of the ALB domain and in sync
func CheckDomainDNSReady(ctx context.Context, domain string, validCNAMEs []string) bool {
	for i, validCNAME := range validCNAMEs {
		validCNAMEs[i] = Normalize(validCNAME)
	}
	cname, err := getCNAMEInSync(ctx, domain)
	Logger.Debugf("CNAME for %v is: '%v', err: %v", domain, cname, err)
	// Ignore other types of DNS errors
	if err == errDNSNotInSync {
		Logger.Debugf("CNAME for %v is not in sync: %v", domain, cname)
		return false
	}
	cname = Normalize(cname)
	if slices.Contains(validCNAMEs, cname) {
		Logger.Debugf("CNAME for %v is in sync: %v", domain, cname)
		return true
	}

	albIPAddrs, err := resolver.LookupIPAddr(ctx, validCNAMEs[0])
	if err != nil {
		Logger.Debugf("Could not resolve A/AAAA record for load balancer %v: %v", validCNAMEs[0], err)
		return false
	}
	albIPs := IpAddrsToIPs(albIPAddrs)

	// In sync CNAME may be pointing to the same IP addresses of the load balancer, considered as valid
	Logger.Debugf("Checking CNAME %v", cname)
	if cname != "" {
		cnameIPAddrs, err := resolver.LookupIPAddr(ctx, cname)
		if err != nil {
			Logger.Debugf("Could not resolve A/AAAA record for %v: %v", cname, err)
		} else {
			Logger.Debugf("IP for %v is %v", cname, cnameIPAddrs)
			cnameIPs := IpAddrsToIPs(cnameIPAddrs)
			if containsAllIPs(albIPs, cnameIPs) {
				Logger.Debugf("CNAME for %v is pointing to %v which has the same IP addresses as the load balancer %v", domain, cname, validCNAMEs)
				return true
			}
		}
	}

	// Check if an valid A record has been set
	ips, err := getIPInSync(ctx, domain)
	if err != nil {
		Logger.Debugf("IP for %v not in sync: %v", domain, err)
		return false
	}
	if containsAllIPs(albIPs, ips) {
		Logger.Debugf("IP for %v is pointing to the same IP addresses as the load balancer %v", domain, validCNAMEs) // TODO: Better warning message
		return true
	}
	return false
}

func getCNAMEInSync(ctx context.Context, domain string) (string, error) {
	ns, err := FindNSServers(ctx, domain)
	if err != nil {
		return "", err
	}

	cnames := make(map[string]bool)
	var cname string
	var lookupErr error
	for _, n := range ns {
		cname, err = ResolverAt(n.Host).LookupCNAME(ctx, domain)
		if err != nil {
			Logger.Debugf("Error looking up CNAME for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		cnames[cname] = true
	}
	if len(cnames) > 1 {
		Logger.Debugf("CNAMEs for %v are not in sync among NS servers %v: %v", domain, NSHosts(ns), cnames)
		return "", errDNSNotInSync
	}
	return cname, lookupErr
}

func getIPInSync(ctx context.Context, domain string) ([]net.IP, error) {
	ns, err := FindNSServers(ctx, domain)
	if err != nil {
		return nil, err
	}

	var results []net.IP
	var lookupErr error
	for i, n := range ns {
		var ipAddrs []net.IPAddr
		ipAddrs, err = ResolverAt(n.Host).LookupIPAddr(ctx, domain)
		if err != nil {
			Logger.Debugf("Error looking up IP for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		if i == 0 {
			for _, ip := range ipAddrs {
				results = append(results, ip.IP)
			}
		} else {
			newFoundIPs := IpAddrsToIPs(ipAddrs)
			if !SameIPs(results, newFoundIPs) {
				Logger.Debugf("IP addresses for %v are not in sync among NS servers %v: %v <> %v", domain, NSHosts(ns), results, newFoundIPs)
				return nil, errDNSNotInSync
			}
		}
	}
	return results, lookupErr
}

func containsAllIPs(all []net.IP, subset []net.IP) bool {
	for _, ip := range subset {
		found := false
		for _, a := range all {
			if a.Equal(ip) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
