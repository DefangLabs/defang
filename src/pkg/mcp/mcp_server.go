package mcp

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	agentTools "github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/mcp/prompts"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	// NewDefangMCPServer returns a new MCPServer instance with all resources, tools, and prompts registered.
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

var SystemInstructions = "Defang provides tools for deploying web applications to cloud providers (AWS, GCP, Digital Ocean) using a compose.yaml file."

type ToolTracker struct {
	providerId *cliClient.ProviderID
	cluster    string
	client     string
}

func (t *ToolTracker) TrackTool(name string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := request.Params.Name
		term.Debug("MCP Tool Called: " + name + " with params: " + fmt.Sprintf("%+v", request.Params))
		track.Evt("MCP Tool Called", track.P("tool", name), track.P("client", t.client), track.P("cluster", t.cluster), track.P("provider", *t.providerId))
		resp, err := handler(ctx, request)
		if err != nil {
			term.Error("MCP Tool Failed: "+name, "error", err)
		} else {
			term.Debug("MCP Tool Succeeded: " + name)
		}
		track.Evt("MCP Tool Done", track.P("tool", name), track.P("client", t.client), track.P("cluster", t.cluster), track.P("provider", *t.providerId), track.P("error", err))
		return resp, err
	}
}

func NewDefangMCPServer(version string, cluster string, providerID *cliClient.ProviderID, client MCPClient, cli agentTools.CLIInterface) (*server.MCPServer, error) {
	// Setup knowledge base
	if err := SetupKnowledgeBase(); err != nil {
		return nil, fmt.Errorf("failed to setup knowledge base: %w", err)
	}

	s := server.NewMCPServer(
		"Deploy with Defang",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
		server.WithInstructions(SystemInstructions),
		server.WithElicitation(),
	)

	defangTools := tools.CollectTools(s, cluster, providerID)
	resources.SetupResources(s)
	prompts.SetupPrompts(s, cluster, providerID)

	// This is used to pass down information of what MCP client we are using
	common.MCPDevelopmentClient = string(client)

	toolTracker := ToolTracker{
		providerId: providerID,
		cluster:    cluster,
		client:     common.MCPDevelopmentClient,
	}
	for i := range defangTools {
		defangTools[i].Handler = toolTracker.TrackTool(defangTools[i].Tool.Name, defangTools[i].Handler)
	}

	s.AddTools(defangTools...)
	return s, nil
}
