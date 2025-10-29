package tools

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// handleLoginTool handles the login tool logic
func handleLoginTool(ctx context.Context, cluster string, authPort int, cli CLIInterface) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		if authPort != 0 {
			return cli.GenerateAuthURL(authPort), nil
		}
		term.Debug("Function invoked: cli.InteractiveLoginPrompt")
		err = cli.InteractiveLoginMCP(ctx, client, cluster, common.MCPDevelopmentClient)
		if err != nil {
			var noBrowserErr auth.ErrNoBrowser
			if errors.As(err, &noBrowserErr) {
				return noBrowserErr.Error(), nil
			}
			return "", err
		}
	}

	output := "Successfully logged in to Defang"

	term.Debug(output)
	return output, nil
}
