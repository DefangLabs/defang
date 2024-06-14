package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func GetServices(ctx context.Context, client client.Client, long bool) error {
	project, err := client.LoadProject(ctx)
	if err != nil {
		return err
	}
	term.Debug("Listing services in project", project.Name)

	serviceList, err := client.GetServices(ctx)
	if err != nil {
		return err
	}

	if !long {
		for _, si := range serviceList.Services {
			*si = defangv1.ServiceInfo{Service: &defangv1.Service{Name: si.Service.Name}}
		}
	}

	PrintObject("", serviceList)
	return nil
}
