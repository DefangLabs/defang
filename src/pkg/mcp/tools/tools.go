package tools

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer, client client.GrpcClient, cluster string, gitHubClientId string) {
	// Create a tool for logging in and getting a new token
	term.Info("Setting up login tool")
	setupLoginTool(s, client, cluster, gitHubClientId)

	// Create a tool for listing services
	term.Info("Setting up services tool")
	setupServicesTool(s, client)

	// Create a tool for deployment
	term.Info("Setting up deployment tool")
	setupDeployTool(s, client)

	// Create a tool for destroying services
	term.Info("Setting up destroy tool")
	setupDestroyTool(s, client)

	term.Info("All MCP tools have been set up successfully")
}
