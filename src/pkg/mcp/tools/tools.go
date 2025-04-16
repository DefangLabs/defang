package tools

import (
	"github.com/DefangLabs/defang/src/pkg/mcp/logger"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer) {
	// Create a tool for logging in and getting a new token
	logger.Sugar.Info("Setting up login tool")
	setupLoginTool(s)

	// Create a tool for listing services
	logger.Sugar.Info("Setting up services tool")
	setupServicesTool(s)

	// Create a tool for deployment
	logger.Sugar.Info("Setting up deployment tool")
	setupDeployTool(s)

	// Create a tool for destroying services
	logger.Sugar.Info("Setting up destroy tool")
	setupDestroyTool(s)

	logger.Sugar.Info("All MCP tools have been set up successfully")
}
