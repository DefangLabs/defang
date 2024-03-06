package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func GetServices(ctx context.Context, client client.Client, long bool) error {
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
