package cli

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoServices struct {
	ProjectName string // may be empty
}

func (e ErrNoServices) Error() string {
	return "no services found in project " + e.ProjectName // ProjectName may be empty
}

func GetServices(ctx context.Context, projectName string, provider client.Provider, long bool) error {
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return err
	}

	numServices := len(servicesResponse.Services)
	if numServices == 0 {
		return ErrNoServices{ProjectName: projectName}
	}

	term.Info("Checking service health...")
	UpdateServiceStates(ctx, servicesResponse.Services)

	return PrintServiceInfos(servicesResponse, long)
}

func PrintServiceInfos(servicesResponse *defangv1.GetServicesResponse, long bool) error {
	if long {
		return PrintObject("", servicesResponse)
	}

	return PrintServiceStatesAndEndpoints(servicesResponse.Services)
}

func UpdateServiceStates(ctx context.Context, serviceInfos []*defangv1.ServiceInfo) {
	// Create a context with a timeout for HTTP requests
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup

	for _, serviceInfo := range serviceInfos {
		for _, endpoint := range serviceInfo.Endpoints {
			if !strings.Contains(endpoint, ":") {
				wg.Add(1)
				go func(serviceInfo *defangv1.ServiceInfo) {
					defer wg.Done()
					url := "https://" + endpoint + serviceInfo.HealthcheckPath
					// Use the regular net/http package to make the request without retries
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
					if err != nil {
						term.Errorf("Failed to create healthcheck request for %q at %s: %s", serviceInfo.Service.Name, url, err.Error())
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
	}
	wg.Wait()
}
