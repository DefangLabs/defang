package tools

import (
	"context"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

// DefaultLoginCLI provides the default implementation
type DefaultLoginCLI struct{}

func (c *DefaultLoginCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultLoginCLI) InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string) error {
	return login.InteractiveLoginMCP(ctx, client, cluster)
}

func (c *DefaultLoginCLI) GenerateAuthURL(authPort int) string {
	return "Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"
}

// handleLoginTool handles the login tool logic
func handleLoginTool(ctx context.Context, request mcp.CallToolRequest, cluster string, authPort int, cli LoginCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Login tool called")
	term.Debugf("mcp.CallToolRequest %+v", request)

	// Test token
	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		if authPort != 0 {
			return mcp.NewToolResultText(cli.GenerateAuthURL(authPort)), nil
		}
		term.Debug("Function invoked: cli.InteractiveLoginPrompt")
		err = cli.InteractiveLoginMCP(ctx, client, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to login", err), err
		}
	}

	output := "Successfully logged in to Defang"

	term.Debug(output)
	return mcp.NewToolResultText(output), nil
}
