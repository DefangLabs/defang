package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/mcp/auth"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupLoginTool configures and adds the login tool to the MCP server
func setupLoginTool(s *server.MCPServer) {
	term.Info("Creating login tool")
	loginTool := mcp.NewTool("login",
		mcp.WithDescription("Login to Defang"),
	)
	term.Debug("Login tool created")

	// Add the login tool handler - make it non-blocking
	term.Info("Adding login tool handler")
	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get a valid token
		_, err := auth.GetValidTokenAndSave(ctx)
		if err != nil {
			term.Error("Failed to get valid token", "error", err)
			return mcp.NewToolResultText(fmt.Sprintf("Failed to get valid token: %v", err)), nil
		}

		output := "Successfully logged in to Defang"

		term.Info(output)
		return mcp.NewToolResultText(output), nil
	})
}
