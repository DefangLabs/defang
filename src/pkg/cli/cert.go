package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/spinner"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var resolver dns.Resolver = dns.RootResolver{}
var httpClient HTTPClient = http.DefaultClient

func GenerateLetsEncryptCert(ctx context.Context, client cliClient.Client) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}

	term.Debug(" - Generating TLS cert for project", projectName)

	services, err := client.GetServices(ctx)
	if err != nil {
		return err
	}

	cnt := 0
	for _, service := range services.Services {
		if service.Service != nil && service.Service.Domainname != "" && service.ZoneId == "" {
			cnt++
			targets := []string{service.PublicFqdn}
			for i, endpoint := range service.Endpoints {
				if service.Service.Ports[i].Mode == defangv1.Mode_INGRESS {
					targets = append(targets, endpoint)
				}
			}
			term.Debugf(" * Found service %v with domain %v and targets %v", service.Service.Name, service.Service.Domainname, targets)
			generateCert(ctx, service.Service.Domainname, targets)
		}
	}
	if cnt == 0 {
		term.Infof(" * No services found need to generate TLS cert")
	}

	return nil
}

func generateCert(ctx context.Context, domain string, targets []string) {
	term.Infof(" * Triggering TLS cert generation for %v", domain)
	if err := waitForCNAME(ctx, domain, targets); err != nil {
		term.Errorf("Error waiting for CNAME: %v", err)
		return
	}

	term.Infof(" * %v DNS is properly configured!", domain)
	if err := checkTLSCert(ctx, domain); err == nil {
		term.Infof(" * TLS cert for %v is already ready", domain)
		return
	}
	if err := pkg.SleepWithContext(ctx, 5*time.Second); err != nil { // slight delay to ensure DNS to propagate
		term.Errorf("Error waiting for DNS propagation: %v", err)
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
	doSpinner := term.StdoutCanColor() && term.IsTerminal()
	if doSpinner {
		spinCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			term.HideCursor()
			defer term.ShowCursor()
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

	doSpinner := term.StdoutCanColor() && term.IsTerminal()
	if doSpinner {
		term.HideCursor()
		defer term.ShowCursor()
	}
	spin := spinner.New()
	for {
		select {
		case <-timeout.Done():
			return timeout.Err()
		case <-ticker.C:
			if err := checkTLSCert(timeout, domain); err == nil {
				return nil
			} else {
				term.Debugf(" - Error checking TLS cert for %v: %v", domain, err)
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

func waitForCNAME(ctx context.Context, domain string, targets []string) error {
	for i, target := range targets {
		targets[i] = strings.TrimSuffix(strings.ToLower(target), ".")
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	msgShown := false
	doSpinner := term.StdoutCanColor() && term.IsTerminal()
	if doSpinner {
		term.HideCursor()
		defer term.ShowCursor()
	}
	spin := spinner.New()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if checkDomainDNSReady(ctx, domain, targets) {
				return nil
			}
			if !msgShown {
				term.Infof(" * Please setup CNAME record for %v", domain)
				fmt.Printf("  %v  CNAME or as an alias to [ %v ]\n", domain, strings.Join(targets, " or "))
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
func checkDomainDNSReady(ctx context.Context, domain string, validCNAMEs []string) bool {
	for i, validCNAME := range validCNAMEs {
		validCNAMEs[i] = strings.TrimSuffix(validCNAME, ".")
	}
	cname, err := getCNAMEInSync(ctx, domain)
	term.Debugf(" - CNAME for %v is :'%v', err: %v", domain, cname, err)
	// Ignore other types of DNS errors
	if err == errDNSNotInSync {
		term.Debugf(" - CNAME for %v is not in sync: %v", domain, cname)
		return false
	}
	cname = strings.TrimSuffix(cname, ".")
	if slices.Contains(validCNAMEs, cname) {
		term.Debugf(" - CNAME for %v is in sync: %v", domain, cname)
		return true
	}

	albIPAddrs, err := resolver.LookupIPAddr(ctx, validCNAMEs[0])
	if err != nil {
		term.Debugf(" - Could not resolve A/AAAA record for load balancer %v: %v", validCNAMEs[0], err)
		return false
	}
	albIPs := dns.IpAddrsToIPs(albIPAddrs)

	// In sync CNAME may be pointing to the same IP addresses of the load balancer, considered as valid
	term.Debugf(" - Checking CNAME %v", cname)
	if cname != "" {
		cnameIPAddrs, err := resolver.LookupIPAddr(ctx, cname)
		if err != nil {
			term.Debugf(" - Could not resolve A/AAAA record for %v: %v", cname, err)
		} else {
			term.Debugf(" - IP for %v is %v", cname, cnameIPAddrs)
			cnameIPs := dns.IpAddrsToIPs(cnameIPAddrs)
			if containsAllIPs(albIPs, cnameIPs) {
				term.Warnf(" ! CNAME for %v is pointing to %v which has the same IP addresses of the load balancer %v", domain, cname, validCNAMEs)
				return true
			}
		}
	}

	// Check if an valid A record has been set
	ips, err := getIPInSync(ctx, domain)
	if err != nil {
		term.Debugf(" - IP for %v not in sync: %v", domain, err)
		return false
	}
	if containsAllIPs(albIPs, ips) {
		term.Warnf(" ! IP for %v is pointing to the same IP addresses of the load balancer %v", domain, validCNAMEs) // TODO: Better warning message
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
		resp, err := httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			var msg []byte
			msg, err = io.ReadAll(resp.Body)
			if err == nil {
				err = fmt.Errorf("HTTP %v: %v", resp.StatusCode, string(msg))
			}
		}

		term.Debugf(" - Error fetching %v: %v, tries left %v", url, err, tries-i-1)
		errs = append(errs, err)

		delay := (100 * time.Millisecond) << i // Simple exponential backoff
		if err := pkg.SleepWithContext(ctx, delay); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

var errDNSNotInSync = errors.New("DNS not in sync")

func getCNAMEInSync(ctx context.Context, domain string) (string, error) {
	ns, err := dns.FindNSServers(ctx, domain)
	if err != nil {
		return "", err
	}

	cnames := make(map[string]bool)
	var cname string
	var lookupErr error
	for _, n := range ns {
		cname, err = dns.ResolverAt(n.Host).LookupCNAME(ctx, domain)
		if err != nil {
			term.Debugf(" - Error looking up CNAME for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		cnames[cname] = true
	}
	if len(cnames) > 1 {
		term.Debugf(" - CNAMEs for %v are not in sync among NS servers %v: %v", domain, dns.NSHosts(ns), cnames)
		return "", errDNSNotInSync
	}
	return cname, lookupErr
}

func getIPInSync(ctx context.Context, domain string) ([]net.IP, error) {
	ns, err := dns.FindNSServers(ctx, domain)
	if err != nil {
		return nil, err
	}

	var results []net.IP
	var lookupErr error
	for i, n := range ns {
		var ipAddrs []net.IPAddr
		ipAddrs, err = dns.ResolverAt(n.Host).LookupIPAddr(ctx, domain)
		if err != nil {
			term.Debugf(" - Error looking up IP for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		if i == 0 {
			for _, ip := range ipAddrs {
				results = append(results, ip.IP)
			}
		} else {
			newFoundIPs := dns.IpAddrsToIPs(ipAddrs)
			if !dns.SameIPs(results, newFoundIPs) {
				term.Debugf(" - IP addresses for %v are not in sync among NS servers %v: %v <> %v", domain, dns.NSHosts(ns), results, newFoundIPs)
				return nil, errDNSNotInSync
			}
		}
	}
	return results, lookupErr
}
