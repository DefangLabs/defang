package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func Restart(ctx context.Context, client client.Client, names ...string) ([]*pb.ServiceInfo, error) {
	Debug(" - Restarting service", names)
	if DoDryRun {
		return nil, nil
	}

	// For now, we'll just get the service info and pass it to deploy as-is.
	services := make([]*pb.Service, 0, len(names))
	for _, name := range names {
		serviceInfo, err := client.Get(ctx, (&pb.ServiceID{Name: name}))
		if err != nil {
			Warn(" ! Failed to get service", name, err)
			continue
		}
		services = append(services, serviceInfo.Service)
	}

	resp, err := client.Deploy(ctx, &pb.DeployRequest{Services: services})
	if err != nil {
		return nil, err
	}
	return resp.Services, nil
}
