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

var resolver = net.Resolver{}
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
	var errs []error
	for _, service := range services.Services {
		if service.Service != nil && service.Service.Domainname != "" && service.ZoneId == "" {
			cnt++
			err := generateCert(ctx, service.Service.Domainname, service.LbDns)
			errs = append(errs, err)
		}
	}
	if cnt == 0 {
		term.Warnf(" ! No services found that use Let's Encrypt cert")
	}

	return errors.Join(errs...)
}

func generateCert(ctx context.Context, domain, albDns string) error {
	term.Infof(" * Triggering Let's Encrypt cert generation for %v", domain)
	if err := waitForCNAME(ctx, domain, albDns); err != nil {
		return fmt.Errorf("Error waiting for CNAME: %w", err)
	}

	term.Infof(" * %v DNS is properly configured!", domain)
	if err := checkTLSCert(ctx, domain); err == nil {
		term.Infof(" * TLS cert for %v is already installed", domain)
		return nil
	}
	term.Infof(" * Triggering cert generation for %v", domain)
	if err := triggerCertGeneration(ctx, domain); err != nil {
		// Ignore possible tls error as cert attachment may take time
		term.Debugf("Error triggering cert generation: %v", err)
	}

	term.Infof(" * Waiting for TLS cert to be online for %v", domain)
	if err := waitForTLS(ctx, domain); err != nil {
		// FIXME: Add more info on how to debug, possibly provided by the server side to avoid client type detection here
		return fmt.Errorf("Error waiting for TLS to be online: %w", err)
	}

	fmt.Printf("TLS cert for %v is ready\n", domain)
	return nil
}

func triggerCertGeneration(ctx context.Context, domain string) error {
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
	return getWithRetries(ctx, fmt.Sprintf("http://%v", domain), 3) // Retry incase of DNS error
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

func waitForCNAME(ctx context.Context, domain, albDns string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	albDns = strings.TrimSuffix(albDns, ".")
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
			cname, err := waitForCNAMEInSync(ctx, domain)
			term.Debugf(" - CNAME for %v is %v: %v", domain, cname, err)
			cname = strings.TrimSuffix(cname, ".")
			if err != nil || !strings.EqualFold(cname, albDns) {
				if !msgShown {
					term.Infof(" * Please setup CNAME record for %v", domain)
					fmt.Printf("  %v  CNAME  %v\n", domain, strings.ToLower(albDns))
					term.Infof(" * Waiting for CNAME record setup and DNS propagation...")
					msgShown = true
				}
				if doSpinner {
					fmt.Print(spin.Next())
				}
			} else {
				return nil
			}
		}
	}
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

func waitForCNAMEInSync(ctx context.Context, domain string) (string, error) {
	ns, err := getNSServers(ctx, domain)
	if err != nil {
		return "", err
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cnames := make(map[string]bool)
			var cname string
			for _, n := range ns {
				cname, err = resolverAt(n).LookupCNAME(ctx, domain)
				if err != nil {
					term.Debugf(" - Error looking up CNAME for %v at %v: %v", domain, n, err)
				}
				cnames[cname] = true
			}
			if len(cnames) > 1 {
				term.Debugf(" - CNAMEs for %v are not in sync: %v", domain, cnames)
				continue
			}
			return cname, err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
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

func resolverAt(nsServer string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, network, nsServer+":53")
		},
	}
}
