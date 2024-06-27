package command

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) {
	// We can only show services deployed to the prod1 defang SaaS environment.
	if provider == cliClient.ProviderDefang && cluster == cli.DefaultCluster {
		term.Info("Monitor your services' status in the defang portal")
		for _, serviceInfo := range serviceInfos {
			fmt.Println("   -", SERVICE_PORTAL_URL+"/"+serviceInfo.Service.Name)
		}
	}
}

func printEndpoints(serviceInfos []*defangv1.ServiceInfo) {
	for _, serviceInfo := range serviceInfos {
		andEndpoints := ""
		if len(serviceInfo.Endpoints) > 0 {
			andEndpoints = "and will be available at:"
		}

		serviceConditionText := "has status " + serviceInfo.Status
		if serviceInfo.State != defangv1.ServiceState_UNKNOWN {
			serviceConditionText = "is in state " + serviceInfo.State.String()
		}

		term.Info("Service", serviceInfo.Service.Name, serviceConditionText, andEndpoints)
		for i, endpoint := range serviceInfo.Endpoints {
			if serviceInfo.Service.Ports[i].Mode == defangv1.Mode_INGRESS {
				endpoint = "https://" + endpoint
			}
			fmt.Println("   -", endpoint)
		}
		if serviceInfo.Service.Domainname != "" {
			if serviceInfo.ZoneId != "" {
				fmt.Println("   -", "https://"+serviceInfo.Service.Domainname)
			} else {
				fmt.Println("   -", "https://"+serviceInfo.Service.Domainname+" (after `defang cert generate` to get a TLS certificate)")
			}
		}
	}
}
