package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ErrNoServices struct {
	ProjectName string // may be empty
}

func (e ErrNoServices) Error() string {
	return fmt.Sprintf("no services found in project %q", e.ProjectName)
}

type PrintService struct {
	Name        string
	Etag        string
	PublicFqdn  string
	PrivateFqdn string
	Status      string
}

func GetServices(ctx context.Context, loader client.Loader, provider client.Provider, long bool) error {
	projectName, err := client.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing services in project %q", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return err
	}

	numServices := len(servicesResponse.Services)

	if numServices == 0 {
		return ErrNoServices{ProjectName: projectName}
	}

	if long {
		// Truncate nanoseconds from timestamps for readability.
		services := make([]*defangv1.ServiceInfo, 0, len(servicesResponse.Services))
		for _, si := range servicesResponse.Services {
			si.CreatedAt = timestamppb.New(si.CreatedAt.AsTime().Truncate(time.Second))
			services = append(services, si)
		}

		servicesResponse.Services = services
		servicesResponse.ExpiresAt = timestamppb.New(servicesResponse.ExpiresAt.AsTime().Truncate(time.Second))
		return PrintObject("", servicesResponse)
	}

	printServices := make([]PrintService, numServices)
	for i, si := range servicesResponse.Services {
		printServices[i] = PrintService{
			Name:        si.Service.Name,
			Etag:        si.Etag,
			PublicFqdn:  si.PublicFqdn,
			PrivateFqdn: si.PrivateFqdn,
			Status:      si.Status,
		}
		servicesResponse.Services[i] = nil
	}

	return term.Table(printServices, []string{"Name", "Etag", "PublicFqdn", "PrivateFqdn", "Status"})
}
