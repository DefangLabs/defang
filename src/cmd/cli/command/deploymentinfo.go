package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const DEFANG_PORTAL_HOST = "portal.defang.dev"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) {
	// We can only show services deployed to the prod1 defang SaaS environment.
	if providerID == cliClient.ProviderDefang && cluster == cli.DefaultCluster {
		term.Info("Monitor your services' status in the defang portal")
		for _, serviceInfo := range serviceInfos {
			term.Println("   -", SERVICE_PORTAL_URL+"/"+serviceInfo.Service.Name)
		}
	}
}

func printServiceStatesAndEndpoints(serviceInfos []*defangv1.ServiceInfo) {
	for _, serviceInfo := range serviceInfos {
		andEndpoints := ""
		if len(serviceInfo.Endpoints) > 0 {
			andEndpoints = "and will be available at:"
		}

		serviceConditionText := "has status " + serviceInfo.Status
		if serviceInfo.State != defangv1.ServiceState_NOT_SPECIFIED {
			serviceConditionText = "is in state " + serviceInfo.State.String()
		}

		term.Info("Service", serviceInfo.Service.Name, serviceConditionText, andEndpoints)
		for i, endpoint := range serviceInfo.Endpoints {
			if serviceInfo.Service.Ports[i].Mode == defangv1.Mode_INGRESS {
				endpoint = "https://" + endpoint
			}
			term.Println("   -", endpoint)
		}
		if serviceInfo.Domainname != "" {
			if serviceInfo.ZoneId != "" {
				term.Println("   -", "https://"+serviceInfo.Domainname)
			} else {
				term.Println("   -", "https://"+serviceInfo.Domainname+" (after `defang cert generate` to get a TLS certificate)")
			}
		}
	}
}
