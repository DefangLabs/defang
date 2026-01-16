package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ServiceLineItem struct {
	Deployment        string
	Endpoint          string
	Service           string
	State             defangv1.ServiceState
	Status            string
	Fqdn              string
	AcmeCertUsed      bool
	HealthcheckStatus string
}

type ErrNoServices struct {
	ProjectName string // may be empty
}

func (e ErrNoServices) Error() string {
	if e.ProjectName == "" {
		return "no services found"
	}
	return fmt.Sprintf("no services found in project %q; check logs for deployment status", e.ProjectName)
}

// This was broken out to avoid breaking the printed output when the --long flag is used
func PrintLongServices(ctx context.Context, projectName string, provider client.Provider) error {
	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return err
	}

	numServices := len(servicesResponse.Services)
	if numServices == 0 {
		return nil
	}
	return PrintObject("", servicesResponse)
}

func GetServices(ctx context.Context, projectName string, provider client.Provider) ([]ServiceLineItem, error) {
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return nil, err
	}

	numServices := len(servicesResponse.Services)
	if numServices == 0 {
		return nil, ErrNoServices{ProjectName: projectName}
	}

	results := GetHealthcheckResults(ctx, servicesResponse.Services)
	services, err := NewServiceFromServiceInfo(servicesResponse.Services)
	if err != nil {
		return nil, err
	}
	for i, svc := range services {
		if status, ok := results[svc.Service]; ok {
			services[i].HealthcheckStatus = *status
		} else {
			services[i].HealthcheckStatus = "unknown"
		}
	}
	return services, nil
}

func PrintServices(ctx context.Context, projectName string, provider client.Provider) error {
	services, err := GetServices(ctx, projectName, provider)
	if err != nil {
		return err
	}

	return PrintServiceStatesAndEndpoints(services)
}

type HealthCheckResults map[string]*string

func GetHealthcheckResults(ctx context.Context, serviceInfos []*defangv1.ServiceInfo) HealthCheckResults {
	// Create a context with a timeout for HTTP requests
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	results := make(HealthCheckResults)
	for _, serviceInfo := range serviceInfos {
		results[serviceInfo.Service.Name] = (new(string))
	}

	for _, serviceInfo := range serviceInfos {
		for _, endpoint := range serviceInfo.Endpoints {
			if strings.Contains(endpoint, ":") {
				*results[serviceInfo.Service.Name] = "skipped"
				// Skip endpoints with ports because they likely non-HTTP services
				continue
			}
			wg.Add(1)
			go func(serviceInfo *defangv1.ServiceInfo) {
				defer wg.Done()
				result, err := RunHealthcheck(ctx, serviceInfo.Service.Name, "https://"+endpoint, serviceInfo.HealthcheckPath)
				if err != nil {
					term.Debugf("Healthcheck error for service %q at endpoint %q: %s", serviceInfo.Service.Name, endpoint, err.Error())
					result = "error"
				}
				*results[serviceInfo.Service.Name] = result
			}(serviceInfo)
		}
	}

	wg.Wait()

	return results
}

func RunHealthcheck(ctx context.Context, name, endpoint, path string) (string, error) {
	url, err := url.JoinPath(endpoint, path)
	if err != nil {
		return "", err
	}
	// Use the regular net/http package to make the request without retries
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	term.Debugf("[%s] checking health at %s", name, url)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		term.Debugf("[%s] ✔ healthy", name)
		return "healthy", nil
	} else {
		term.Debugf("[%s] ✘ unhealthy (%s)", name, resp.Status)
		return "unhealthy (" + resp.Status + ")", nil
	}
}

func NewServiceFromServiceInfo(serviceInfos []*defangv1.ServiceInfo) ([]ServiceLineItem, error) {
	var serviceTableItems []ServiceLineItem

	// showDomainNameColumn := false

	for _, serviceInfo := range serviceInfos {
		fqdn := serviceInfo.PublicFqdn
		if fqdn == "" {
			fqdn = serviceInfo.PrivateFqdn
		}
		domainname := "N/A"
		if serviceInfo.Domainname != "" {
			// showDomainNameColumn = true
			domainname = "https://" + serviceInfo.Domainname
		} else if serviceInfo.PublicFqdn != "" {
			domainname = "https://" + serviceInfo.PublicFqdn
		} else if serviceInfo.PrivateFqdn != "" {
			domainname = serviceInfo.PrivateFqdn
		}

		ps := ServiceLineItem{
			Deployment:   serviceInfo.Etag,
			Service:      serviceInfo.Service.Name,
			State:        serviceInfo.State,
			Status:       serviceInfo.Status,
			Endpoint:     domainname,
			Fqdn:         fqdn,
			AcmeCertUsed: serviceInfo.UseAcmeCert,
		}
		serviceTableItems = append(serviceTableItems, ps)
	}

	return serviceTableItems, nil
}

func PrintServiceStatesAndEndpoints(services []ServiceLineItem) error {
	showCertGenerateHint := false
	printHealthcheckStatus := false
	for _, svc := range services {
		if svc.AcmeCertUsed {
			showCertGenerateHint = true
		}
		if svc.HealthcheckStatus != "" {
			printHealthcheckStatus = true
		}
	}

	attrs := []string{"Service", "Deployment", "State", "Fqdn", "Endpoint"}
	if printHealthcheckStatus {
		attrs = append(attrs, "HealthcheckStatus")
	}
	// if showDomainNameColumn {
	// 	attrs = append(attrs, "DomainName")
	// }

	err := term.Table(services, attrs...)
	if err != nil {
		return err
	}

	if showCertGenerateHint {
		term.Info("Run `defang cert generate` to get a TLS certificate for your service(s)")
	}

	return nil
}
