package mcp

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/mcp/prompts"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/mark3labs/mcp-go/server"

	// NewDefangMCPServer returns a new MCPServer instance with all resources, tools, and prompts registered.
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

func prepareInstructions(defangTools []server.ServerTool) string {
	instructions := "Defang provides tools for deploying web applications to cloud providers (AWS, GCP, Digital Ocean) using a compose.yaml file."
	for _, tool := range defangTools {
		instructions += "\n\n" + tool.Tool.Name + " - " + tool.Tool.Description
	}
	return instructions
}

func NewDefangMCPServer(version string, cluster string, authPort int, providerID *cliClient.ProviderID) (*server.MCPServer, error) {
	// Setup knowledge base
	if err := SetupKnowledgeBase(); err != nil {
		return nil, fmt.Errorf("failed to setup knowledge base: %w", err)
	}

	defangTools := tools.CollectTools(cluster, authPort, providerID)
	s := server.NewMCPServer(
		"Deploy with Defang",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithInstructions(prepareInstructions(defangTools)),
	)

	resources.SetupResources(s)
	prompts.SetupPrompts(s, cluster, providerID)

	s.AddTools(defangTools...)
	return s, nil
}
