package cli

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type printService struct {
	Deployment string
	Endpoint   string
	Service    string
	State      defangv1.ServiceState
	Status     string
	Fqdn       string
}

func PrintServiceStatesAndEndpoints(serviceInfos []*defangv1.ServiceInfo) error {
	var serviceTableItems []*printService

	// showDomainNameColumn := false
	showCertGenerateHint := false

	for _, serviceInfo := range serviceInfos {
		fqdn := serviceInfo.PublicFqdn
		if fqdn == "" {
			fqdn = serviceInfo.PrivateFqdn
		}
		domainname := "N/A"
		if serviceInfo.Domainname != "" {
			// showDomainNameColumn = true
			domainname = "https://" + serviceInfo.Domainname
			if serviceInfo.UseAcmeCert {
				showCertGenerateHint = true
			}
		} else if serviceInfo.PublicFqdn != "" {
			domainname = "https://" + serviceInfo.PublicFqdn
		} else if serviceInfo.PrivateFqdn != "" {
			domainname = serviceInfo.PrivateFqdn
		}

		ps := &printService{
			Deployment: serviceInfo.Etag,
			Service:    serviceInfo.Service.Name,
			State:      serviceInfo.State,
			Status:     serviceInfo.Status,
			Endpoint:   domainname,
			Fqdn:       fqdn,
		}
		serviceTableItems = append(serviceTableItems, ps)
	}

	attrs := []string{"Service", "Deployment", "State", "Fqdn", "Endpoint", "Status"}
	// if showDomainNameColumn {
	// 	attrs = append(attrs, "DomainName")
	// }

	err := term.Table(serviceTableItems, attrs...)
	if err != nil {
		return err
	}

	if showCertGenerateHint {
		term.Info("Run `defang cert generate` to get a TLS certificate for your service(s)")
	}

	return nil
}
