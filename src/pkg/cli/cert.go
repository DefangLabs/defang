package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cert"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
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
			term.Debugf("Redirecting from %v to %v", via[len(via)-1].URL, req.URL)
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
	term.Debugf("Generating TLS cert for project %q", projectName)

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
			term.Debugf("Found service %v with domain %v and targets %v", service.Service.Name, service.Service.Domainname, targets)
			generateCert(ctx, service.Service.Domainname, targets, client)
		}
	}
	if cnt == 0 {
		term.Infof("No HTTPS services found; no TLS cert generation needed")
	}

	return nil
}

func generateCert(ctx context.Context, domain string, targets []string, client cliClient.Client) {
	term.Infof("Triggering TLS cert generation for %v", domain)
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
	}
	spin := spinner.New()
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
			if doSpinner {
				fmt.Print(spin.Next())
			}
		}
	}
}

func waitForCNAME(ctx context.Context, domain string, targets []string, client cliClient.Client) error {
	for i, target := range targets {
		targets[i] = strings.TrimSuffix(strings.ToLower(target), ".")
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	msgShown := false
	serverSideVerified := false
	serverVerifyRpcFailure := 0
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
			} else {
				locallyVerified := dns.CheckDomainDNSReady(ctx, domain, targets)
				if serverSideVerified && !locallyVerified {
					term.Warnf("The DNS configuration for %v has been successfully verified. However, your local environment may still be using cached data, so it could take several minutes for the DNS changes to propagate on your system.", domain)
					return nil
				}
				if locallyVerified {
					return nil
				}
			}
			if !msgShown {
				term.Infof("Please set up a CNAME record for %v", domain)
				fmt.Printf("  %v  CNAME or as an alias to [ %v ]\n", domain, strings.Join(targets, " or "))
				term.Infof("Waiting for CNAME record setup and DNS propagation...")
				msgShown = true
			}
			if doSpinner {
				fmt.Print(spin.Next())
			}
		}
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

		term.Debugf("Error fetching %v: %v, tries left %v", url, err, tries-i-1)
		errs = append(errs, err)

		delay := httpRetryDelayBase << i // Simple exponential backoff
		if err := pkg.SleepWithContext(ctx, delay); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}
