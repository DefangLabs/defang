package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func Restart(ctx context.Context, client client.Client, names ...string) ([]*v1.ServiceInfo, error) {
	Debug(" - Restarting service", names)

	// For now, we'll just get the service info and pass it to deploy as-is.
	services := make([]*v1.Service, 0, len(names))
	for _, name := range names {
		serviceInfo, err := client.Get(ctx, (&v1.ServiceID{Name: name}))
		if err != nil {
			Warn(" ! Failed to get service", name, err)
			continue
		}
		services = append(services, serviceInfo.Service)
	}

	if DoDryRun {
		return nil, ErrDryRun
	}

	resp, err := client.Deploy(ctx, &v1.DeployRequest{Services: services})
	if err != nil {
		return nil, err
	}
	return resp.Services, nil
}
