package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Restart(ctx context.Context, client defangv1connect.FabricControllerClient, names ...string) ([]*v1.ServiceInfo, error) {
	Debug(" - Restarting service", names)
	if DoDryRun {
		return nil, nil
	}

	serviceInfos := make([]*v1.ServiceInfo, 0, len(names))
	for _, name := range names {
		Info(" * Restarting service", name)
		serviceInfo, err := client.Get(ctx, connect.NewRequest(&v1.ServiceID{Name: name}))
		if err != nil {
			Warn(" ! Failed to get service", name, err)
			continue
		}
		serviceInfo, err = client.Update(ctx, connect.NewRequest(serviceInfo.Msg.Service))
		if err != nil {
			// Abort all? TODO: restart should probably be atomic, all or nothing
			if len(serviceInfos) == 0 {
				return nil, err
			}
			Warn(" ! Failed to restart service", name, err)
			continue
		}
		serviceInfos = append(serviceInfos, serviceInfo.Msg)
	}
	return serviceInfos, nil
}
