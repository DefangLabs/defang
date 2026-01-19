package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ServiceEndpoint struct {
	Deployment      string
	Endpoint        string
	Service         string
	State           string
	Status          string
	AcmeCertUsed    bool
	HealthcheckPath string
	Healthcheck     string // status
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

func GetServices(ctx context.Context, projectName string, provider client.Provider) ([]ServiceEndpoint, error) {
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return nil, err
	}

	serviceInfos := servicesResponse.Services
	numServices := len(serviceInfos)
	if numServices == 0 {
		return nil, ErrNoServices{ProjectName: projectName}
	}

	serviceEndpoints, err := ServiceEndpointsFromServiceInfos(serviceInfos)
	if err != nil {
		return nil, err
	}
	UpdateHealthcheckResults(ctx, serviceEndpoints)
	return serviceEndpoints, nil
}

func PrintServices(ctx context.Context, projectName string, provider client.Provider) error {
	services, err := GetServices(ctx, projectName, provider)
	if err != nil {
		return err
	}

	return PrintServiceStatesAndEndpoints(services)
}

func UpdateHealthcheckResults(ctx context.Context, serviceEndpoints []ServiceEndpoint) {
	// Create a context with a timeout for HTTP requests
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	for i, serviceEndpoint := range serviceEndpoints {
		if strings.Contains(serviceEndpoint.Endpoint, ":") && !strings.HasPrefix(serviceEndpoint.Endpoint, "https://") {
			serviceEndpoints[i].Healthcheck = "-"
			continue
		}
		wg.Add(1)
		go func(serviceEndpoint *ServiceEndpoint) {
			defer wg.Done()
			result, err := RunHealthcheck(ctx, serviceEndpoint.Service, serviceEndpoint.Endpoint, serviceEndpoint.HealthcheckPath)
			if err != nil {
				term.Debugf("Healthcheck error for service %q at endpoint %q: %s", serviceEndpoint.Service, serviceEndpoint.Endpoint, err.Error())
				result = "error"
			}
			serviceEndpoint.Healthcheck = result
		}(&serviceEndpoints[i])
	}

	wg.Wait()
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
		if errors.Is(err, context.DeadlineExceeded) {
			return "unknown (timeout)", nil
		}
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			term.Warnf("service %q: Run `defang cert generate` to continue setup: %v", name, err)
			return "unknown (DNS error)", nil
		}
		var tlsErr *tls.CertificateVerificationError
		if errors.As(err, &tlsErr) {
			term.Warnf("service %q: Run `defang cert generate` to continue setup: %v", name, err)
			return "unknown (TLS certificate error)", nil
		}
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

func ServiceEndpointsFromServiceInfo(serviceInfo *defangv1.ServiceInfo) []ServiceEndpoint {
	endpoints := make([]ServiceEndpoint, 0, len(serviceInfo.Endpoints)+1)
	for _, endpoint := range serviceInfo.Endpoints {
		_, port, _ := net.SplitHostPort(endpoint)
		if port == "" {
			endpoint = "https://" + strings.TrimPrefix(endpoint, "https://")
		}
		endpoints = append(endpoints, ServiceEndpoint{
			Deployment:      serviceInfo.Etag,
			Service:         serviceInfo.Service.Name,
			State:           serviceInfo.State.String(),
			Status:          serviceInfo.Status,
			Endpoint:        endpoint,
			HealthcheckPath: serviceInfo.HealthcheckPath,
			AcmeCertUsed:    serviceInfo.UseAcmeCert,
		})
	}
	if serviceInfo.Domainname != "" {
		endpoints = append(endpoints, ServiceEndpoint{
			Deployment:      serviceInfo.Etag,
			Service:         serviceInfo.Service.Name,
			State:           serviceInfo.State.String(),
			Status:          serviceInfo.Status,
			Endpoint:        "https://" + serviceInfo.Domainname,
			HealthcheckPath: serviceInfo.HealthcheckPath,
			AcmeCertUsed:    serviceInfo.UseAcmeCert,
		})
	}
	return endpoints
}

func ServiceEndpointsFromServiceInfos(serviceInfos []*defangv1.ServiceInfo) ([]ServiceEndpoint, error) {
	var serviceTableItems []ServiceEndpoint

	for _, serviceInfo := range serviceInfos {
		serviceTableItems = append(serviceTableItems, ServiceEndpointsFromServiceInfo(serviceInfo)...)
	}

	return serviceTableItems, nil
}

func PrintServiceStatesAndEndpoints(serviceEndpoints []ServiceEndpoint) error {
	showCertGenerateHint := false
	printHealthcheckStatus := false
	for _, svc := range serviceEndpoints {
		if svc.AcmeCertUsed {
			showCertGenerateHint = true
		}
		if svc.Healthcheck != "" {
			printHealthcheckStatus = true
		}
	}

	attrs := []string{"Service", "Deployment", "State"}
	if printHealthcheckStatus {
		attrs = append(attrs, "Healthcheck", "Endpoint")
	} else {
		attrs = append(attrs, "Endpoint")
	}

	// sort serviceEndpoints by Service, Deployment, Endpoint
	slices.SortStableFunc(serviceEndpoints, func(a, b ServiceEndpoint) int {
		if a.Service != b.Service {
			return strings.Compare(a.Service, b.Service)
		}
		if a.Deployment != b.Deployment {
			return strings.Compare(a.Deployment, b.Deployment)
		}
		return strings.Compare(a.Endpoint, b.Endpoint)
	})

	// remove "Service", "Deployment", and "State" columns if they are the same as the previous row
	lastService := ""
	for i := range serviceEndpoints {
		if serviceEndpoints[i].Service == lastService {
			serviceEndpoints[i].Service = ""
			serviceEndpoints[i].Deployment = ""
			serviceEndpoints[i].State = ""
		} else {
			lastService = serviceEndpoints[i].Service
		}
	}
	err := term.Table(serviceEndpoints, attrs...)
	if err != nil {
		return err
	}

	if showCertGenerateHint {
		term.Info("Run `defang cert generate` to get a TLS certificate for your service(s)")
	}

	return nil
}
