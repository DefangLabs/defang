package tools

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer) {
	// Create a tool for logging in and getting a new token
	term.Info("Setting up login tool")
	setupLoginTool(s)

	// Create a tool for listing services
	term.Info("Setting up services tool")
	setupServicesTool(s)

	// Create a tool for deployment
	term.Info("Setting up deployment tool")
	setupDeployTool(s)

	// Create a tool for destroying services
	term.Info("Setting up destroy tool")
	setupDestroyTool(s)

	term.Info("All MCP tools have been set up successfully")
}
