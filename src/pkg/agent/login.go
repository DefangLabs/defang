package agent

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type LoginParams struct{}

type LoginCLIInterface interface {
	Connecter
	// Unique methods
	InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string) error
	GenerateAuthURL(authPort int) string
}

func MakeLoginToolHandler(cluster string, authPort int, cli LoginCLIInterface) func(ctx context.Context, params LoginParams) (string, error) {
	return func(ctx context.Context, params LoginParams) (string, error) {
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			if authPort != 0 {
				return cli.GenerateAuthURL(authPort), nil
			}
			term.Debug("Function invoked: cli.InteractiveLoginPrompt")
			err = cli.InteractiveLoginMCP(ctx, client, cluster)
			if err != nil {
				return "", fmt.Errorf("Failed to login", err)
			}
		}

		return "Successfully logged in to Defang", nil
	}
}
