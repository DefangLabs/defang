package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cert"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/spinner"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type DNSResult struct {
	IPs    []net.IPAddr
	Expiry time.Time
}

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
				rootAddr := net.JoinHostPort(ips[pkg.RandomIndex(len(ips))].String(), port)
				return dialer.DialContext(ctx, network, rootAddr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			term.Debugf("Redirecting from %v to %v", via[len(via)-1].URL, req.URL)
			return nil
		},
	}
	httpRetryDelayBase = 5 * time.Second
)

func GenerateLetsEncryptCert(ctx context.Context, project *compose.Project, client client.FabricClient, provider client.Provider) error {
	term.Debugf("Generating TLS cert for project %q", project.Name)

	services, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: project.Name})
	if err != nil {
		return err
	}

	// First, check if there are any domain names in the compose file at all
	hasDomains := false
	for _, service := range project.Services {
		if service.DomainName != "" {
			hasDomains = true
			break
		}
	}

	cnt := 0
	for _, serviceInfo := range services.Services {
		if service, ok := project.Services[serviceInfo.Service.Name]; ok && service.DomainName != "" && serviceInfo.ZoneId == "" {
			cnt++
			targets := getDomainTargets(serviceInfo, service)
			domains := []string{service.DomainName}
			if defaultNetwork := service.Networks["default"]; defaultNetwork != nil {
				domains = append(domains, defaultNetwork.Aliases...)
			}
			term.Debugf("Found service %v with domains %v and targets %v", service.Name, domains, targets)
			for _, domain := range domains {
				generateCert(ctx, domain, targets, client)
			}
		}
	}
	// Only show the "no domainname found" message if there truly are no domains in the compose file
	if cnt == 0 && !hasDomains {
		term.Infof("No `domainname` found in compose file; no HTTPS cert generation needed")
	}

	return nil
}

func getDomainTargets(serviceInfo *defangv1.ServiceInfo, service compose.ServiceConfig) []string {
	// Only use the ALB for aws cert gen to avoid defang domain in the middle
	if serviceInfo.LbDnsName != "" {
		return []string{serviceInfo.LbDnsName}
	} else {
		targets := []string{serviceInfo.PublicFqdn}
		for i, endpoint := range serviceInfo.Endpoints {
			if service.Ports[i].Mode == compose.Mode_INGRESS {
				targets = append(targets, endpoint)
			}
		}
		return targets
	}
}
func generateCert(ctx context.Context, domain string, targets []string, client client.FabricClient) {
	term.Infof("Checking DNS setup for %v", domain)
	if err := waitForCNAME(ctx, domain, targets, client); err != nil {
		term.Errorf("Error waiting for CNAME: %v", err)
		return
	}

	term.Infof("%v DNS is properly configured!", domain)
	if err := cert.CheckTLSCert(ctx, domain); err == nil {
		term.Infof("TLS cert for %v is already ready", domain)
		return
	}
	if err := pkg.SleepWithContext(ctx, 5*time.Second); err != nil { // slight delay to ensure DNS to propagate
		term.Errorf("Error waiting for DNS propagation: %v", err)
		return
	}
	term.Infof("Triggering cert generation for %v", domain)
	if err := triggerCertGeneration(ctx, domain); err != nil {
		term.Errorf("Error triggering cert generation, please try again")
		return
	}

	term.Infof("Waiting for TLS cert to be online for %v, this could take a few minutes", domain)
	if err := waitForTLS(ctx, domain); err != nil {
		term.Errorf("Error waiting for TLS to be online: %v", err)
		// FIXME: Add more info on how to debug, possibly provided by the server side to avoid client type detection here
		return
	}

	term.Infof("TLS cert for %v is ready\n", domain)
}

func triggerCertGeneration(ctx context.Context, domain string) error {
	doSpinner := term.StdoutCanColor() && term.IsTerminal()
	if doSpinner {
		term.HideCursor()
		defer term.ShowCursor()

		spin := spinner.New()
		cancelSpinner := spin.Start(ctx)
		defer cancelSpinner()
	}
	// Our own retry logic uses the root resolver to prevent cached DNS and retry on all non-200 errors
	if err := getWithRetries(ctx, fmt.Sprintf("http://%v", domain), 5); err != nil { // Retry incase of DNS error
		// Ignore possible tls error as cert attachment may take time
		term.Debugf("Error triggering cert generation: %v", err)
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

		spin := spinner.New()
		cancelSpinner := spin.Start(ctx)
		defer cancelSpinner()
	}
	for {
		select {
		case <-timeout.Done():
			return timeout.Err()
		case <-ticker.C:
			if err := cert.CheckTLSCert(timeout, domain); err == nil {
				return nil
			} else {
				term.Debugf("Error checking TLS cert for %v: %v", domain, err)
			}
		}
	}
}

func waitForCNAME(ctx context.Context, domain string, targets []string, client client.FabricClient) error {
	for i, target := range targets {
		targets[i] = dns.Normalize(target)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	serverSideVerified := false
	serverVerifyRpcFailure := 0
	doSpinner := term.StdoutCanColor() && term.IsTerminal()
	if doSpinner {
		term.HideCursor()
		defer term.ShowCursor()

		spin := spinner.New()
		cancelSpinner := spin.Start(ctx)
		defer cancelSpinner()
	}

	verifyDNS := func() error {
		if !serverSideVerified && serverVerifyRpcFailure < 3 {
			if err := client.VerifyDNSSetup(ctx, &defangv1.VerifyDNSSetupRequest{Domain: domain, Targets: targets}); err == nil {
				term.Debugf("Server side DNS verification for %v successful", domain)
				serverSideVerified = true
			} else {
				if cerr := new(connect.Error); errors.As(err, &cerr) && cerr.Code() == connect.CodeFailedPrecondition {
					term.Debugf("Server side DNS verification negative result: %v", cerr.Message())
				} else {
					term.Debugf("Server side DNS verification request for %v failed: %v", domain, err)
					serverVerifyRpcFailure++
				}
			}
			if serverVerifyRpcFailure >= 3 {
				term.Warnf("Server side DNS verification for %v failed multiple times, skipping server side DNS verification.", domain)
			}
		}
		if serverSideVerified || serverVerifyRpcFailure >= 3 {
			locallyVerified := dns.CheckDomainDNSReady(ctx, domain, targets)
			if serverSideVerified && !locallyVerified {
				term.Warnf("DNS settings for %v are verified, but changes may take a few minutes to propagate due to caching.", domain)
				return nil
			}
			if locallyVerified {
				return nil
			}
		}
		return errors.New("not verified")
	}

	if err := verifyDNS(); err == nil {
		return nil
	}
	term.Infof("Configure a CNAME or ALIAS record for the domain name: %v", domain)
	fmt.Printf("  %v  -> %v\n", domain, strings.Join(targets, " or "))
	term.Infof("Awaiting DNS record setup and propagation... This may take a while.")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := verifyDNS(); err == nil {
				return nil
			}
		}
	}
}

func getWithRetries(ctx context.Context, url string, tries int) error {
	var errs []error
	for i := range make([]struct{}, tries) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err // No point retrying if we can't even create the request
		}
		resp, err := httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body) // Read the body to ensure the request is not swallowed by alb
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			if resp != nil && resp.Request != nil && resp.Request.URL.Scheme == "https" {
				term.Debugf("cert gen request success, received redirect to %v", resp.Request.URL)
				return nil // redirect to https indicate a successful cert generation
			}
			if err == nil {
				err = fmt.Errorf("HTTP: %v", resp.StatusCode)
			}
		} else if cve := new(tls.CertificateVerificationError); errors.As(err, &cve) {
			term.Debugf("cert gen request success, received tls error: %v", cve)
			return nil // tls error indicate a successful cert gen trigger, as it has to be redirected to https
		}

		term.Debugf("Error fetching %v: %v, tries left %v", url, err, tries-i-1)
		errs = append(errs, err)

		delay := httpRetryDelayBase << i // Simple exponential backoff
		if err := pkg.SleepWithContext(ctx, delay); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}
