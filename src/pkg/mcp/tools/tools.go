package tools

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer, cluster string, authPort int, providerId *client.ProviderID) {
	// Create a tool for logging in and getting a new token
	term.Debug("Setting up login tool")
	setupLoginTool(s, cluster, authPort)

	// Create a tool for listing services
	term.Debug("Setting up services tool")
	setupServicesTool(s, cluster, providerId)

	// Create a tool for deployment
	term.Debug("Setting up deployment tool")
	setupDeployTool(s, cluster, providerId)

	// Create a tool for destroying services
	term.Debug("Setting up destroy tool")
	setupDestroyTool(s, cluster, providerId)

	// Create a tool for estimating costs
	term.Debug("Setting up estimate tool")
	setupEstimateTool(s, cluster, providerId)

	// Create a tool to set config variables
	term.Debug("Setting up set config tool")
	setupSetConfigTool(s, cluster, providerId)

	// Create a tool to remove config variables
	term.Debug("Setting up remove config tool")
	setupRemoveConfigTool(s, cluster, providerId)

	// Create a tool to list config variables
	term.Debug("Setting up list config tool")
	setupListConfigTool(s, cluster, providerId)

	term.Debug("All MCP tools have been set up successfully")
}
