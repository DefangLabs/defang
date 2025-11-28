package command

import (
	"regexp"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	pcluster "github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/globals"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const DEFANG_PORTAL_HOST = "portal.defang.io"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) {
	// We can only show services deployed to the prod1 defang SaaS environment.
	if globals.Config.ProviderID == cliClient.ProviderDefang && globals.Config.Cluster == pcluster.DefaultCluster {
		term.Info("Monitor your services' status in the defang portal")
		for _, serviceInfo := range serviceInfos {
			term.Println("   -", SERVICE_PORTAL_URL+"/"+serviceInfo.Service.Name)
		}
	}
}

type ServiceTableItem struct {
	Deployment string `json:"Deployment"`
	Status     string `json:"Status"`
	Name       string `json:"Name"`
	DomainName string `json:"DomainName"`
	Endpoints  string `json:"Endpoints"`
}

func printServiceStatesAndEndpoints(serviceInfos []*defangv1.ServiceInfo) error {
	serviceTableItems := make([]ServiceTableItem, 0, len(serviceInfos))

	showDomainNameColumn := false
	showCertGenerateHint := false
	hasPort := regexp.MustCompile(`:\d{1,5}$`)

	for _, serviceInfo := range serviceInfos {
		var domainname string
		if serviceInfo.Domainname != "" {
			showDomainNameColumn = true
			domainname = "https://" + serviceInfo.Domainname
			if serviceInfo.ZoneId == "" {
				showCertGenerateHint = true
			}
		}
		endpoints := make([]string, 0, len(serviceInfo.Endpoints))
		for _, endpoint := range serviceInfo.Endpoints {
			if !hasPort.MatchString(endpoint) {
				endpoint = "https://" + endpoint
			}

			endpoints = append(endpoints, endpoint)
		}
		if len(endpoints) == 0 {
			endpoints = append(endpoints, "N/A")
		}

		serviceTableItems = append(serviceTableItems, ServiceTableItem{
			Deployment: serviceInfo.Etag,
			Name:       serviceInfo.Service.Name,
			Status:     serviceInfo.State.String(),
			DomainName: domainname,
			Endpoints:  strings.Join(endpoints, ", "),
		})
	}

	attrs := []string{"Deployment", "Name", "Status", "Endpoints"}
	if showDomainNameColumn {
		attrs = append(attrs, "DomainName")
	}

	err := term.Table(serviceTableItems, attrs...)
	if err != nil {
		return err
	}

	if showCertGenerateHint {
		term.Info("Run `defang cert generate` to get a TLS certificate for your service(s)")
	}

	return nil
}
