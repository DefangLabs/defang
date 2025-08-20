package tools

import (
	"context"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupLoginTool configures and adds the login tool to the MCP server
func setupLoginTool(s *server.MCPServer, cluster string, authPort int) {
	term.Debug("Creating login tool")
	loginTool := mcp.NewTool("login",
		mcp.WithDescription("Login to Defang"),
	)
	term.Debug("Login tool created")

	// Add the login tool handler - make it non-blocking
	term.Debug("Adding login tool handler")
	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Debug("Login tool called")
		// Test token
		term.Debug("Function invoked: cli.Connect")
		track.Evt("MCP Login Tool")

		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			if authPort != 0 {
				return mcp.NewToolResultText("Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"), nil
			}
			term.Debug("Function invoked: cli.InteractiveLoginPrompt")
			err = login.InteractiveLoginMCP(ctx, client, cluster)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("Failed to login", err), nil
			}
		}

		output := "Successfully logged in to Defang"

		term.Debug(output)
		return mcp.NewToolResultText(output), nil
	})
}
