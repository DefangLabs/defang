package tools

import (
	"context"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupLoginTool configures and adds the login tool to the MCP server
func setupLoginTool(s *server.MCPServer, cluster string, authPort int) {
	term.Info("Creating login tool")
	loginTool := mcp.NewTool("login",
		mcp.WithDescription("Login to Defang"),
	)
	term.Debug("Login tool created")

	// Add the login tool handler - make it non-blocking
	term.Info("Adding login tool handler")
	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Infof("Login tool called")
		// Test token
		term.Debug("Function invoked: cli.Connect")
		client, err := cli.Connect(ctx, cluster)
		client.Track("MCP Login Tool")
		if err != nil {
			if authPort != 0 {
				return mcp.NewToolResultText("Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"), nil
			}
			term.Debug("Function invoked: cli.InteractiveLoginPrompt")
			err = cli.InteractiveLoginPrompt(ctx, client, cluster)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("Failed to login", err), nil
			}
		}

		output := "Successfully logged in to Defang"

		term.Info(output)
		return mcp.NewToolResultText(output), nil
	})
}
