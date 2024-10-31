package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var ErrNoServices = errors.New("no services found")

func GetServices(ctx context.Context, loader compose.Loader, provider client.Provider, long bool) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing services in project %q", projectName)

	serviceList, err := provider.GetServices(ctx, projectName)
	if err != nil {
		return err
	}

	if len(serviceList.Services) == 0 {
		return ErrNoServices
	}

	if !long {
		for _, si := range serviceList.Services {
			*si = defangv1.ServiceInfo{Service: &defangv1.Service{Name: si.Service.Name}}
		}
	}

	PrintObject("", serviceList)
	return nil
}
