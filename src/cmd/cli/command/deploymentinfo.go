package command

import (
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const DEFANG_PORTAL_HOST = "portal.defang.dev"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) {
	// We can only show services deployed to the prod1 defang SaaS environment.
	if provider == cliClient.ProviderDefang && cluster == cli.DefaultCluster {
		term.Info("Monitor your services' status in the defang portal")
		for _, serviceInfo := range serviceInfos {
			term.Println("   -", SERVICE_PORTAL_URL+"/"+serviceInfo.Service.Name)
		}
	}
}

func printEndpoints(serviceInfos []*defangv1.ServiceInfo) {
	for _, serviceInfo := range serviceInfos {
		andEndpoints := ""
		if len(serviceInfo.Endpoints) > 0 {
			andEndpoints = "and will be available at:"
		}
		term.Info("Service", serviceInfo.Service.Name, "is in state", serviceInfo.Status, andEndpoints)
		for _, endpoint := range serviceInfo.Endpoints {
			if url, ok := strings.CutSuffix(endpoint, ":443"); ok {
				endpoint = "https://" + url
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
