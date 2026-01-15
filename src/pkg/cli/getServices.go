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

type Service struct {
	Deployment   string
	Endpoint     string
	Service      string
	State        defangv1.ServiceState
	Status       string
	Fqdn         string
	AcmeCertUsed bool
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

func GetServices(ctx context.Context, projectName string, provider client.Provider) (*defangv1.GetServicesResponse, error) {
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return nil, err
	}

	numServices := len(servicesResponse.Services)
	if numServices == 0 {
		return nil, ErrNoServices{ProjectName: projectName}
	}

	UpdateServiceStates(ctx, servicesResponse.Services)

	return servicesResponse, nil
}

func PrintServices(ctx context.Context, projectName string, provider client.Provider, long bool) error {
	servicesResponse, err := GetServices(ctx, projectName, provider)
	if err != nil {
		return err
	}
	if long {
		return PrintObject("", servicesResponse)
	}

	services, err := GetServiceStatesAndEndpoints(servicesResponse.Services)
	if err != nil {
		return err
	}

	return PrintServiceStatesAndEndpoints(services)
}

func UpdateServiceStates(ctx context.Context, serviceInfos []*defangv1.ServiceInfo) {
	// Create a context with a timeout for HTTP requests
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	for _, serviceInfo := range serviceInfos {
		for _, endpoint := range serviceInfo.Endpoints {
			if strings.Contains(endpoint, ":") {
				// Skip endpoints with ports because they likely non-HTTP services
				continue
			}
			wg.Add(1)
			go func(serviceInfo *defangv1.ServiceInfo) {
				defer wg.Done()
				url, err := url.JoinPath("https://"+endpoint, serviceInfo.HealthcheckPath)
				if err != nil {
					term.Errorf("failed to construct healthcheck URL for %q at endpoint %s: %s", serviceInfo.Service.Name, endpoint, err.Error())
					return
				}
				// Use the regular net/http package to make the request without retries
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				if err != nil {
					term.Errorf("failed to create healthcheck request for %q at %s: %s", serviceInfo.Service.Name, url, err.Error())
					return
				}
				term.Debugf("[%s] checking health at %s", serviceInfo.Service.Name, url)
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					term.Errorf("Healthcheck failed for %q at %s: %s", serviceInfo.Service.Name, url, err.Error())
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					serviceInfo.State = defangv1.ServiceState_DEPLOYMENT_COMPLETED
					term.Debugf("[%s] ✔ healthy", serviceInfo.Service.Name)
				} else {
					term.Debugf("[%s] ✘ unhealthy (%s)", serviceInfo.Service.Name, resp.Status)
				}
			}(serviceInfo)
		}
	}
	wg.Wait()
}

func GetServiceStatesAndEndpoints(serviceInfos []*defangv1.ServiceInfo) ([]Service, error) {
	var serviceTableItems []Service

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

		ps := Service{
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

func PrintServiceStatesAndEndpoints(services []Service) error {
	showCertGenerateHint := false
	for _, svc := range services {
		if svc.AcmeCertUsed {
			showCertGenerateHint = true
			break
		}
	}

	attrs := []string{"Service", "Deployment", "State", "Fqdn", "Endpoint", "Status"}
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
