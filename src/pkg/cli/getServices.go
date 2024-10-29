package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var ErrNoServices = errors.New("no services found")

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
