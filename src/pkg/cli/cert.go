package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
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

type DNSResult struct {
	IPs    []net.IPAddr
	Expiry time.Time
}

type logger interface {
	Debugf(format string, args ...any) (int, error)
	Infof(format string, args ...any) (int, error)
	Warnf(format string, args ...any) (int, error)
	Errorf(format string, args ...any) (int, error)
}

var Logger logger = term.DefaultTerm

var (
	resolver         dns.Resolver = dns.RootResolver{}
	dnsCache                      = make(map[string]DNSResult)
	dnsCacheDuration              = 1 * time.Minute
	httpClient       HTTPClient   = &http.Client{
		// Based on the default transport: https://pkg.go.dev/net/http#RoundTripper
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				cached, ok := dnsCache[host]
				var ips []net.IPAddr
				if ok && cached.Expiry.After(time.Now()) {
					ips = cached.IPs
				} else {
					ips, err = resolver.LookupIPAddr(ctx, host)
					if err != nil {
						return nil, err
					}
					// Keep 1min of dns cache to avoid spamming root dns servers
					expiry := time.Now().Add(dnsCacheDuration)
					dnsCache[host] = DNSResult{ips, expiry}
				}

				dialer := &net.Dialer{}
				rootAddr := net.JoinHostPort(ips[rand.Intn(len(ips))].String(), port)
				return dialer.DialContext(ctx, network, rootAddr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			Logger.Debugf("Redirecting from %v to %v", via[len(via)-1].URL, req.URL)
			return nil
		},
	}
	httpRetryDelayBase = 5 * time.Second
)

func GenerateLetsEncryptCert(ctx context.Context, client cliClient.Client) error {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	Logger.Debugf("Generating TLS cert for project %q", projectName)

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
			Logger.Debugf("Found service %v with domain %v and targets %v", service.Service.Name, service.Service.Domainname, targets)
			generateCert(ctx, service.Service.Domainname, targets, client)
		}
	}
	if cnt == 0 {
		Logger.Infof("No HTTPS services found; no TLS cert generation needed")
	}

	return nil
}

func generateCert(ctx context.Context, domain string, targets []string, client cliClient.Client) {
	Logger.Infof("Triggering TLS cert generation for %v", domain)
	if err := waitForCNAME(ctx, domain, targets, client); err != nil {
		Logger.Errorf("Error waiting for CNAME: %v", err)
		return
	}

	Logger.Infof("%v DNS is properly configured!", domain)
	if err := CheckTLSCert(ctx, domain); err == nil {
		Logger.Infof("TLS cert for %v is already ready", domain)
		return
	}
	if err := pkg.SleepWithContext(ctx, 5*time.Second); err != nil { // slight delay to ensure DNS to propagate
		Logger.Errorf("Error waiting for DNS propagation: %v", err)
		return
	}
	Logger.Infof("Triggering cert generation for %v", domain)
	if err := triggerCertGeneration(ctx, domain); err != nil {
		Logger.Errorf("Error triggering cert generation, please try again")
		return
	}

	Logger.Infof("Waiting for TLS cert to be online for %v, this could take a few minutes", domain)
	if err := waitForTLS(ctx, domain); err != nil {
		Logger.Errorf("Error waiting for TLS to be online: %v", err)
		// FIXME: Add more info on how to debug, possibly provided by the server side to avoid client type detection here
		return
	}

	Logger.Infof("TLS cert for %v is ready\n", domain)
}

func triggerCertGeneration(ctx context.Context, domain string) error {
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
	// Our own retry logic uses the root resolver to prevent cached DNS and retry on all non-200 errors
	if err := getWithRetries(ctx, fmt.Sprintf("http://%v", domain), 5); err != nil { // Retry incase of DNS error
		// Ignore possible tls error as cert attachment may take time
		Logger.Debugf("Error triggering cert generation: %v", err)
		return err
	}
	return nil
}

func waitForTLS(ctx context.Context, domain string) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	timeout, cancel := context.WithTimeout(ctx, 10*time.Minute)
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
			if err := CheckTLSCert(timeout, domain); err == nil {
				return nil
			} else {
				Logger.Debugf("Error checking TLS cert for %v: %v", domain, err)
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

func waitForCNAME(ctx context.Context, domain string, targets []string, client cliClient.Client) error {
	for i, target := range targets {
		targets[i] = strings.TrimSuffix(strings.ToLower(target), ".")
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	msgShown := false
	serverSideVerified := false
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
			if !serverSideVerified {
				if err := client.VerifyDNSSetup(ctx, &defangv1.VerifyDNSSetupRequest{Domain: domain, Targets: targets}); err == nil {
					Logger.Debugf("Server side DNS verification for %v successful", domain)
					serverSideVerified = true
				} else {
					Logger.Debugf("Server side DNS verification for %v failed: %v", domain, err)
				}
			} else {
				if !CheckDomainDNSReady(ctx, domain, targets) {
					Logger.Warnf("The DNS configuration for %v has been successfully verified. However, your local environment may still be using cached data, so it could take several minutes for the DNS changes to propagate on your system.", domain)
				}
				return nil
			}
			if !msgShown {
				Logger.Infof("Please set up a CNAME record for %v", domain)
				fmt.Printf("  %v  CNAME or as an alias to [ %v ]\n", domain, strings.Join(targets, " or "))
				Logger.Infof("Waiting for CNAME record setup and DNS propagation...")
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
func CheckDomainDNSReady(ctx context.Context, domain string, validCNAMEs []string) bool {
	for i, validCNAME := range validCNAMEs {
		validCNAMEs[i] = strings.TrimSuffix(validCNAME, ".")
	}
	cname, err := getCNAMEInSync(ctx, domain)
	Logger.Debugf("CNAME for %v is: '%v', err: %v", domain, cname, err)
	// Ignore other types of DNS errors
	if err == errDNSNotInSync {
		Logger.Debugf("CNAME for %v is not in sync: %v", domain, cname)
		return false
	}
	cname = strings.TrimSuffix(cname, ".")
	if slices.Contains(validCNAMEs, cname) {
		Logger.Debugf("CNAME for %v is in sync: %v", domain, cname)
		return true
	}

	albIPAddrs, err := resolver.LookupIPAddr(ctx, validCNAMEs[0])
	if err != nil {
		Logger.Debugf("Could not resolve A/AAAA record for load balancer %v: %v", validCNAMEs[0], err)
		return false
	}
	albIPs := dns.IpAddrsToIPs(albIPAddrs)

	// In sync CNAME may be pointing to the same IP addresses of the load balancer, considered as valid
	Logger.Debugf("Checking CNAME %v", cname)
	if cname != "" {
		cnameIPAddrs, err := resolver.LookupIPAddr(ctx, cname)
		if err != nil {
			Logger.Debugf("Could not resolve A/AAAA record for %v: %v", cname, err)
		} else {
			Logger.Debugf("IP for %v is %v", cname, cnameIPAddrs)
			cnameIPs := dns.IpAddrsToIPs(cnameIPAddrs)
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

func CheckTLSCert(ctx context.Context, domain string) error {
	ips, err := resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		url := fmt.Sprintf("https://%v", domain)
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
			var msg []byte
			msg, err = io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			if err == nil {
				err = fmt.Errorf("HTTP %v: %v", resp.StatusCode, string(msg))
			}
		}

		Logger.Debugf("Error fetching %v: %v, tries left %v", url, err, tries-i-1)
		errs = append(errs, err)

		delay := httpRetryDelayBase << i // Simple exponential backoff
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
			Logger.Debugf("Error looking up CNAME for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		cnames[cname] = true
	}
	if len(cnames) > 1 {
		Logger.Debugf("CNAMEs for %v are not in sync among NS servers %v: %v", domain, dns.NSHosts(ns), cnames)
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
			Logger.Debugf("Error looking up IP for %v at %v: %v", domain, n, err)
			lookupErr = err
		}
		if i == 0 {
			for _, ip := range ipAddrs {
				results = append(results, ip.IP)
			}
		} else {
			newFoundIPs := dns.IpAddrsToIPs(ipAddrs)
			if !dns.SameIPs(results, newFoundIPs) {
				Logger.Debugf("IP addresses for %v are not in sync among NS servers %v: %v <> %v", domain, dns.NSHosts(ns), results, newFoundIPs)
				return nil, errDNSNotInSync
			}
		}
	}
	return results, lookupErr
}
