package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/spinner"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type Resolver interface {
	LookupIPAddr(ctx context.Context, domain string) ([]net.IPAddr, error)
	LookupCNAME(ctx context.Context, domain string) (string, error)
	LookupNS(ctx context.Context, domain string) ([]*net.NS, error)
}

var resolverAt = func(nsServer string) Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, network, nsServer+":53")
		},
	}
}

var resolver Resolver = &net.Resolver{}
var httpClient = http.Client{}

func GenerateLetsEncryptCert(ctx context.Context, client cliClient.Client) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}

	term.Debug(" - Generating Let's Encrypt cert for project", projectName)

	services, err := client.GetServices(ctx)
	if err != nil {
		return err
	}

	cnt := 0
	for _, service := range services.Services {
		if service.Service != nil && service.Service.Domainname != "" && service.ZoneId == "" {
			cnt++
			generateCert(ctx, service.Service.Domainname, service.LbDns)
		}
	}
	if cnt == 0 {
		term.Infof(" * No services found need to generate Let's Encrypt cert")
	}

	return nil
}

func generateCert(ctx context.Context, domain, albDns string) {
	term.Infof(" * Triggering Let's Encrypt cert generation for %v", domain)
	if err := waitForCNAME(ctx, domain, albDns); err != nil {
		term.Errorf("Error waiting for CNAME: %v", err)
		return
	}

	term.Infof(" * %v DNS is properly configured!", domain)
	if err := checkTLSCert(ctx, domain); err == nil {
		term.Infof(" * TLS cert for %v is already ready", domain)
		return
	}
	term.Infof(" * Triggering cert generation for %v", domain)
	triggerCertGeneration(ctx, domain)

	term.Infof(" * Waiting for TLS cert to be online for %v", domain)
	if err := waitForTLS(ctx, domain); err != nil {
		term.Errorf("Error waiting for TLS to be online: %v", err)
		// FIXME: Add more info on how to debug, possibly provided by the server side to avoid client type detection here
		return
	}

	fmt.Printf("TLS cert for %v is ready\n", domain)
}

func triggerCertGeneration(ctx context.Context, domain string) {
	doSpinner := term.CanColor && term.IsTerminal
	if doSpinner {
		spinCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			term.Stdout.HideCursor()
			defer term.Stdout.ShowCursor()
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			spin := spinner.New()
			for {
				select {
				case <-spinCtx.Done():
					return
				case <-ticker.C:
					fmt.Print(spin.Next())
				}
			}
		}()
	}
	if err := getWithRetries(ctx, fmt.Sprintf("http://%v", domain), 3); err != nil { // Retry incase of DNS error
		// Ignore possible tls error as cert attachment may take time
		term.Debugf("Error triggering cert generation: %v", err)
	}
}

func waitForTLS(ctx context.Context, domain string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	doSpinner := term.CanColor && term.IsTerminal
	if doSpinner {
		term.Stdout.HideCursor()
		defer term.Stdout.ShowCursor()
	}
	spin := spinner.New()
	for {
		select {
		case <-timeout.Done():
			return timeout.Err()
		case <-ticker.C:
			if err := checkTLSCert(timeout, domain); err == nil {
				return nil
			}
			if doSpinner {
				fmt.Print(spin.Next())
			}
		}
	}
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

func waitForCNAME(ctx context.Context, domain, albDns string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	msgShown := false
	doSpinner := term.CanColor && term.IsTerminal
	if doSpinner {
		term.Stdout.HideCursor()
		defer term.Stdout.ShowCursor()
	}
	spin := spinner.New()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if checkDomainDNSReady(ctx, domain, albDns) {
				return nil
			}
			if !msgShown {
				term.Infof(" * Please setup CNAME record for %v", domain)
				fmt.Printf("  %v  CNAME  %v\n", domain, strings.ToLower(albDns))
				term.Infof(" * Waiting for CNAME record setup and DNS propagation...")
				msgShown = true
			}
			if doSpinner {
				fmt.Print(spin.Next())
			}
		}
	}
}

// The DNS is considered ready if the CNAME of the domain is pointing to the ALB domain and in sync
// OR if the A record of the domain is pointing to the same IP addresses of the ALB domain and in sync
func checkDomainDNSReady(ctx context.Context, domain, expectedCNAME string) bool {
	expectedCNAME = strings.TrimSuffix(expectedCNAME, ".")
	cname, err := getCNAMEInSync(ctx, domain)
	term.Debugf(" - CNAME for %v is %v: %v", domain, cname, err)
	// Ignore other types of DNS errors
	if err == DNSNotInSyncError {
		return false
	}
	cname = strings.TrimSuffix(cname, ".")
	if strings.EqualFold(cname, expectedCNAME) {
		return true
	}

	// Check if an valid A record has been set
	albIPAddrs, err := resolver.LookupIPAddr(ctx, expectedCNAME)
	if err != nil {
		return false
	}
	albIPs := ipAddrsToIPs(albIPAddrs)

	ips, err := getIPInSync(ctx, domain)
	if err != nil {
		return false
	}
	if containsAllIPs(albIPs, ips) {
		term.Warnf(" * IP for %v is pointing to the same IP addresses of the ALB domain %v", domain, expectedCNAME) // TODO: Better warning message
		return true
	}
	return false
}

func checkTLSCert(ctx context.Context, domain string) error {
	return getWithRetries(ctx, fmt.Sprintf("https://%v", domain), 3)
}

func getWithRetries(ctx context.Context, url string, tries int) error {
	var errs []error
	for i := 0; i < tries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err // No point retrying if we can't even create the request
		}
		if _, err := httpClient.Do(req); err != nil {
			errs = append(errs, err)
		}

		delay := (100 * time.Millisecond) >> i // Simple exponential backoff
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.Join(errs...)
}

var DNSNotInSyncError = errors.New("DNS not in sync")

func getCNAMEInSync(ctx context.Context, domain string) (string, error) {
	ns, err := getNSServers(ctx, domain)
	if err != nil {
		return "", err
	}

	cnames := make(map[string]bool)
	var cname string
	var lookupErr error
	for _, n := range ns {
		cname, err = resolverAt(n).LookupCNAME(ctx, domain)
		if err != nil {
			term.Debugf(" - Error looking up CNAME for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		cnames[cname] = true
	}
	if len(cnames) > 1 {
		term.Debugf(" - CNAMEs for %v are not in sync amoung NS servers %v: %v", domain, ns, cnames)
		return "", DNSNotInSyncError
	}
	return cname, lookupErr
}

func getIPInSync(ctx context.Context, domain string) ([]net.IP, error) {
	ns, err := getNSServers(ctx, domain)
	if err != nil {
		return nil, err
	}

	var results []net.IP
	var lookupErr error
	for i, n := range ns {
		var ipAddrs []net.IPAddr
		ipAddrs, err = resolverAt(n).LookupIPAddr(ctx, domain)
		if err != nil {
			term.Debugf(" - Error looking up IP for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		if i == 0 {
			for _, ip := range ipAddrs {
				results = append(results, ip.IP)
			}
		} else {
			newFoundIPs := ipAddrsToIPs(ipAddrs)
			if !sameIPs(results, newFoundIPs) {
				term.Debugf(" - IP addresses for %v are not in sync amoung NS servers %v: %v <> %v", domain, ns, results, newFoundIPs)
				return nil, DNSNotInSyncError
			}
		}
	}
	return results, lookupErr
}

func getNSServers(ctx context.Context, domain string) ([]string, error) {
	d := domain
	var ns []*net.NS
	for {
		var err error
		ns, err = resolver.LookupNS(ctx, d)
		var ne *net.DNSError
		if errors.As(err, &ne) {
			if strings.Count(d, ".") <= 1 {
				return nil, fmt.Errorf("no DNS server found")
			}
			d = d[strings.Index(domain, ".")+1:]
			continue
		} else if err != nil {
			return nil, fmt.Errorf("failed to find NS server for %v at %v: %v", domain, d, err)
		}
		break
	}
	servers := make([]string, len(ns))
	for i, n := range ns {
		servers[i] = n.Host
	}
	return servers, nil
}

func ipAddrsToIPs(ipAddrs []net.IPAddr) []net.IP {
	ips := make([]net.IP, len(ipAddrs))
	for i, ipAddr := range ipAddrs {
		ips[i] = ipAddr.IP
	}
	return ips
}

func sameIPs(a, b []net.IP) bool {
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
