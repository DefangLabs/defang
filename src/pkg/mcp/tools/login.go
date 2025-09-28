package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

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
			return mcp.NewToolResultErrorFromErr("Failed to login", err), nil
		}
	}

	output := "Successfully logged in to Defang"

	term.Debug(output)
	return mcp.NewToolResultText(output), nil
}
