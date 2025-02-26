package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const DEFANG_PORTAL_HOST = "portal.defang.io"
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

type ServiceTableItem struct {
	Id         string `json:"Id"`
	State      string `json:"State"`
	Name       string `json:"Name"`
	DomainName string `json:"DomainName"`
}

func printServiceStatesAndEndpoints(serviceInfos []*defangv1.ServiceInfo) error {
	serviceTableItems := make([]ServiceTableItem, 0, len(serviceInfos))

	for _, serviceInfo := range serviceInfos {
		var domainname string
		if serviceInfo.Domainname != "" {
			domainname = "https://" + serviceInfo.Domainname
			if serviceInfo.ZoneId == "" {
				domainname = serviceInfo.Domainname + " (after `defang cert generate` to get a TLS certificate)"
			}
		}
		serviceTableItems = append(serviceTableItems, ServiceTableItem{
			Id:         serviceInfo.Etag,
			Name:       serviceInfo.Service.Name,
			State:      serviceInfo.State.String(),
			DomainName: domainname,
		})
	}

	return term.Table(serviceTableItems, []string{"Id", "Name", "State", "DomainName"})
}
