package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
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

		term.Info(fmt.Sprintf("Service: %v", service))

		// Pick all the domains matching acme cert conditions: Has domainname but no zoneId
		// if service.Service == nil && service.Service.Domainname != "" && service.ZoneId == "" {
		// 	wg.Add(1)
		// 	go func(domain string) {
		// 		defer wg.Done()
		// 		generateCert(ctx, domain)
		// 	}(service.Service.Domainname)
		// }
	}
	wg.Wait()

	return nil
}

func checkTLSCert(ctx context.Context, domain string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://"+domain, nil)
	if err != nil {
		return err
	}
	if _, err := http.DefaultClient.Do(req); err != nil {
		return err
	}
	return nil
}

func generateCert(ctx context.Context, domain string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := resolver.LookupHost(ctx, domain); err != nil {
				term.Info(fmt.Sprintf("Error looking up domain: %T, %v", err, err))
			} else if err := checkTLSCert(ctx, domain); err != nil {
				term.Info(fmt.Sprintf("Error checking TLS cert: %T, %v", err, err))
			}
		default:
		}
	}
}
