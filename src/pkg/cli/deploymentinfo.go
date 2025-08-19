package cli

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type printService struct {
	Deployment string
	Endpoint   string
	Fqdn       string // this is needed to set up any DNS records
	Service    string
	State      defangv1.ServiceState
	Status     string
}

func PrintServiceStatesAndEndpoints(ctx context.Context, serviceInfos []*defangv1.ServiceInfo) error {
	var serviceTableItems []*printService

	// showDomainNameColumn := false
	showCertGenerateHint := false

	// Create a context with a timeout for HTTP requests
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	for _, serviceInfo := range serviceInfos {
		fqdn := serviceInfo.PublicFqdn
		if fqdn == "" {
			fqdn = serviceInfo.PrivateFqdn
		}

		endpoint := "N/A"
		if serviceInfo.Domainname != "" {
			// showDomainNameColumn = true
			endpoint = "https://" + serviceInfo.Domainname
			if serviceInfo.UseAcmeCert {
				showCertGenerateHint = true
			}
		} else if serviceInfo.PublicFqdn != "" {
			endpoint = "https://" + serviceInfo.PublicFqdn
		} else if serviceInfo.PrivateFqdn != "" {
			endpoint = serviceInfo.PrivateFqdn
		}

		ps := &printService{
			Deployment: serviceInfo.Etag,
			Service:    serviceInfo.Service.Name,
			State:      serviceInfo.State,
			Status:     serviceInfo.Status,
			Endpoint:   endpoint,
			Fqdn:       fqdn,
		}

		if len(serviceInfo.Endpoints) == 0 {
			serviceTableItems = append(serviceTableItems, ps)
			continue
		}

		for i, endpoint := range serviceInfo.Endpoints {
			if i > 0 {
				ps = &printService{} // reset
			}
			ps.Endpoint = endpoint
			if !strings.Contains(endpoint, ":") {
				ps.Endpoint = "https://" + endpoint

				wg.Add(1)
				go func(ps *printService) {
					defer wg.Done()
					url := ps.Endpoint + serviceInfo.Healthcheck
					// Use the regular net/http package to make the request without retries
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
					if err != nil {
						ps.Status = err.Error()
						return
					}
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						ps.Status = err.Error()
						return
					}
					defer resp.Body.Close()
					ps.Status = resp.Status
				}(ps)
			}

			serviceTableItems = append(serviceTableItems, ps)
		}
	}
	wg.Wait()

	attrs := []string{"Service", "Deployment", "State", "Fqdn", "Endpoint", "Status"}
	// if showDomainNameColumn {
	// 	attrs = append(attrs, "DomainName")
	// }

	err := term.Table(serviceTableItems, attrs)
	if err != nil {
		return err
	}

	if showCertGenerateHint {
		term.Println("Run `defang cert generate` to get a TLS certificate for your service(s)")
	}

	return nil
}
