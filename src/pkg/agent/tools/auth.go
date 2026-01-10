package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func GetClientWithRetry(ctx context.Context, cli CLIInterface, config StackConfig) (*client.GrpcClient, error) {
	client, err := cli.Connect(ctx, config.Cluster)
	if err != nil {
		err = cli.InteractiveLoginMCP(ctx, config.Cluster, common.MCPDevelopmentClient)
		if err != nil {
			return nil, err
		}

		// Reconnect with the new token
		client, err = cli.Connect(ctx, config.Cluster)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}
