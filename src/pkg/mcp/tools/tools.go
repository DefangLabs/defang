package tools

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer, cluster string, authPort int) {
	// Create a tool for logging in and getting a new token
	term.Debug("Setting up login tool")
	setupLoginTool(s, cluster, authPort)

	// Create a tool for listing services
	term.Debug("Setting up services tool")
	setupServicesTool(s, cluster)

	// Create a tool for deployment
	term.Debug("Setting up deployment tool")
	setupDeployTool(s, cluster)

	// Create a tool for destroying services
	term.Debug("Setting up destroy tool")
	setupDestroyTool(s, cluster)

	// Create a tool for estimating costs
	term.Debug("Setting up estimate tool")
	setupEstimateTool(s, cluster)

	// Create a tool to set environment variables and secrets
	term.Debug("Setting up config tool")
	setupConfigTool(s, cluster)

	term.Debug("All MCP tools have been set up successfully")
}
