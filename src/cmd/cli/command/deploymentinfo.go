package command

import (
	"strings"

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
	Endpoints  string `json:"Endpoints"`
}

func printServiceStatesAndEndpoints(serviceInfos []*defangv1.ServiceInfo) error {
	serviceTableItems := make([]ServiceTableItem, 0, len(serviceInfos))

	showDomainNameColumn := false
	showCertGenerateHint := false
	for _, serviceInfo := range serviceInfos {
		var domainname string
		if serviceInfo.Domainname != "" {
			showDomainNameColumn = true
			domainname = "https://" + serviceInfo.Domainname
			if serviceInfo.ZoneId == "" {
				showCertGenerateHint = true
			}
		}
		serviceTableItems = append(serviceTableItems, ServiceTableItem{
			Id:         serviceInfo.Etag,
			Name:       serviceInfo.Service.Name,
			State:      serviceInfo.State.String(),
			DomainName: domainname,
			Endpoints:  strings.Join(serviceInfo.Endpoints, ", "),
		})
	}

	var attrs []string
	if showDomainNameColumn {
		attrs = []string{"Id", "Name", "State", "Endpoints", "DomainName"}
	} else {
		attrs = []string{"Id", "Name", "State", "Endpoints"}
	}

	err := term.Table(serviceTableItems, attrs)
	if err != nil {
		return err
	}

	if showCertGenerateHint {
		term.Info("Run `defang cert generate` to get a TLS certificate for your service(s)")
	}

	return nil
}
