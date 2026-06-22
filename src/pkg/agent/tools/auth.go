package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func GetClientWithRetry(ctx context.Context, cli CLIInterface, fabricAddr string) (*client.GrpcClient, error) {
	client, err := cli.Connect(ctx, fabricAddr)
	if err != nil {
		err = cli.InteractiveLoginMCP(ctx, fabricAddr, common.MCPDevelopmentClient)
		if err != nil {
			return nil, err
		}

		// Reconnect with the new token
		client, err = cli.Connect(ctx, fabricAddr)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}
