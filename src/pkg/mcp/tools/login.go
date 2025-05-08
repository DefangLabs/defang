package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupLoginTool configures and adds the login tool to the MCP server
func setupLoginTool(s *server.MCPServer, cluster string) {
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
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			// return mcp.NewToolResultErrorFromErr("Could not connect", err), nil
		}

		err = cli.InteractiveLoginPrompt(ctx, client, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to login", err), nil
		}

		output := "Successfully logged in to Defang"

		term.Info(output)
		return mcp.NewToolResultText(output), nil
	})
}
