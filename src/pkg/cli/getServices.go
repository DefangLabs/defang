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

func GetServices(ctx context.Context, loader client.Loader, provider client.Provider, long bool) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing services in project %q", projectName)

	serviceList, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return err
	}

	if len(serviceList.Services) == 0 {
		return ErrNoServices{ProjectName: projectName}
	}

	if !long {
		for _, si := range serviceList.Services {
			*si = defangv1.ServiceInfo{Service: &defangv1.Service{Name: si.Service.Name}}
		}
	}

	PrintObject("", serviceList)
	return nil
}
