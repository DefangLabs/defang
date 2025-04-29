package tools

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer, cluster string, gitHubClientId string) {
	// Create a tool for logging in and getting a new token
	term.Info("Setting up login tool")
	setupLoginTool(s, cluster, gitHubClientId)

	// Create a tool for listing services
	term.Info("Setting up services tool")
	setupServicesTool(s, cluster)

	// Create a tool for deployment
	term.Info("Setting up deployment tool")
	setupDeployTool(s, cluster)

	// Create a tool for destroying services
	term.Info("Setting up destroy tool")
	setupDestroyTool(s, cluster)

	term.Info("All MCP tools have been set up successfully")
}
