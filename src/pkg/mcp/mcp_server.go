package mcp

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/mcp/prompts"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"

	// NewDefangMCPServer returns a new MCPServer instance with all resources, tools, and prompts registered.
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

func NewDefangMCPServer(version string, cluster string, authPort int, providerID *cliClient.ProviderID) (*server.MCPServer, error) {
	instructions := `Defang provides tools for deploying web applications to cloud providers (AWS, GCP, Digital Ocean) using a compose.yaml file.

There are a number of available tools to help with deployment, configuration, and manage applications deployed with Defang.

deploy - This tool deploys a web application to the cloud using the compose.yaml file in the application's working directory.
destroy - This tool spins down and removes a deployed project from the cloud, cleaning up all associated resources.
estimate - This tool estimates the cost of running a deployed application based on its resource usage and cloud provider pricing.
services - This tool lists all running services for a deployed application, providing status and resource usage information
list_configs - This tool lists all configuration variables for a deployed application, allowing you to view current settings.
remove_config - This tool removes a configuration variable for a deployed application, allowing you to clean up unused settings.
set_config - This tool sets or updates configuration variables for a deployed application, allowing you to manage environment variables and secrets.`

	// Setup knowledge base
	term.Debug("Setting up knowledge base")
	if err := SetupKnowledgeBase(); err != nil {
		return nil, fmt.Errorf("failed to setup knowledge base: %w", err)
	}

	s := server.NewMCPServer(
		"Deploy with Defang",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithLogging(),
		server.WithElicitation(),
		server.WithInstructions(instructions),
	)

	// Setup resources
	term.Debug("Setting up resources")
	resources.SetupResources(s)

	//setup prompts
	term.Debug("Setting up prompts")
	prompts.SetupPrompts(s, cluster, providerID)

	// Setup tools
	term.Debug("Setting up tools")
	tools.SetupTools(s, cluster, authPort, providerID)
	return s, nil
}
