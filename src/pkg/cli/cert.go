package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	cliClient "github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
)

var resolver = net.Resolver{}
var httpClient = http.Client{}

func GenerateLetsEncryptCert(ctx context.Context, client cliClient.Client) error {
	services, err := client.GetServices(ctx)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, service := range services.Services {
		if service.Service != nil && service.Service.Domainname != "" && service.ZoneId == "" {
			wg.Add(1)
			go func(domain string, albDns string) {
				defer wg.Done()
				generateCert(ctx, domain, albDns)
			}(service.Service.Domainname, service.LbDns)
		}
	}
	wg.Wait()

	return nil
}

func generateCert(ctx context.Context, domain, albDns string) {
	term.Infof("Triggering Let's Encrypt cert generation for %v", domain)
	if err := waitForCNAME(ctx, domain, albDns); err != nil {
		term.Errorf("Error waiting for CNAME: %v", err)
		return
	}

	term.Infof("CNAME record for %v is ready", domain)
	if err := checkTLSCert(ctx, domain); err == nil {
		term.Infof("TLS cert for %v is already ready", domain)
		return
	}
	term.Infof("Triggering cert generation for %v", domain)

	if err := getWithContext(ctx, fmt.Sprintf("http://%v", domain)); err != nil {
		term.Errorf("Error triggering cert generation: %v", err)
		return
	}

	term.Infof("Waiting for TLS cert to be online for %v", domain)
	if err := waitForTLS(ctx, domain); err != nil {
		term.Errorf("Error waiting for TLS: %v", err)
		return
	}

	term.Infof("TLS cert for %v is ready", domain)
}

func waitForTLS(ctx context.Context, domain string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	for {
		select {
		case <-timeout.Done():
			return timeout.Err()
		case <-ticker.C:
			if err := checkTLSCert(timeout, domain); err == nil {
				return nil
			}
		}
	}
}

func waitForCNAME(ctx context.Context, domain, albDns string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	albDns = strings.TrimSuffix(albDns, ".")
	msgShown := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cname, err := resolver.LookupCNAME(ctx, domain)
			cname = strings.TrimSuffix(cname, ".")
			if err != nil || strings.ToLower(cname) != strings.ToLower(albDns) {
				if !msgShown {
					term.Infof("Please setup CNAME record for %v to point to ALB %v", domain, albDns)
					msgShown = true
				}
			} else {
				return nil
			}
		}
	}
}

func checkTLSCert(ctx context.Context, domain string) error {
	return getWithContext(ctx, fmt.Sprintf("https://%v", domain))
}

func getWithContext(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if _, err := http.DefaultClient.Do(req); err != nil {
		return err
	}
	return nil
}
