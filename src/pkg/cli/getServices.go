package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrNoServices struct {
	ProjectName string
}

func (e ErrNoServices) Error() string {
	return fmt.Sprintf("no services found in project %q", e.ProjectName)
}

func GetServices(ctx context.Context, provider client.Provider, long bool) error {
	projectName, err := provider.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Listing services in project %q", projectName)

	serviceList, err := provider.GetServices(ctx)
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
