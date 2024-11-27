package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoServices struct {
	ProjectName string // may be empty
}

func (e ErrNoServices) Error() string {
	return "no services found in project " + e.ProjectName
}

type PrintService struct {
	Name        string
	Etag        string
	PublicFqdn  string
	PrivateFqdn string
	Status      string
}

func GetServices(ctx context.Context, loader client.Loader, provider client.Provider, long bool) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return err
	}

	if len(servicesResponse.Services) == 0 {
		return ErrNoServices{ProjectName: projectName}
	}

	if long {
		return PrintObject("", servicesResponse)
	} else {
		printServices := make([]PrintService, 0, len(servicesResponse.Services))
		for i, si := range servicesResponse.Services {
			printServices = append(printServices, PrintService{
				Name:        si.Service.Name,
				Etag:        si.Etag,
				PublicFqdn:  si.PublicFqdn,
				PrivateFqdn: si.PrivateFqdn,
				Status:      si.Status,
			})
			servicesResponse.Services[i] = nil
		}

		return term.Table(printServices, []string{"Name", "Etag", "PublicFqdn", "PrivateFqdn", "Status"})
	}
}
