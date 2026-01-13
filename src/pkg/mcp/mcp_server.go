package mcp

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	agentTools "github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func prepareInstructions() string {
	instructions := "Defang provides tools for deploying web applications to cloud providers (AWS, GCP, Digital Ocean) using a compose.yaml file."
	return instructions
}

type ToolTracker struct {
	providerId *client.ProviderID
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

type StackConfig = tools.StackConfig

// NewDefangMCPServer returns a new MCPServer instance with all resources, tools registered.
func NewDefangMCPServer(version string, client MCPClient, cli agentTools.CLIInterface, config StackConfig) (*server.MCPServer, error) {
	// Setup knowledge base
	if err := SetupKnowledgeBase(); err != nil {
		return nil, fmt.Errorf("failed to setup knowledge base: %w", err)
	}

	var elicitationsController *elicitations.Controller

	s := server.NewMCPServer(
		"Defang Version",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithToolCapabilities(true),
		server.WithElicitation(),
		server.WithInstructions(prepareInstructions()),
		server.WithHooks(&server.Hooks{
			OnAfterInitialize: []server.OnAfterInitializeFunc{
				func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult) {
					if elicitationsController == nil {
						return
					}

					if message.Params.Capabilities.Elicitation == nil {
						(*elicitationsController).SetSupported(false)
					}
				},
			},
		}),
	)

	resources.SetupResources(s)

	// This is used to pass down information of what MCP client we are using
	common.MCPDevelopmentClient = string(client)

	providerID := config.Stack.Provider
	toolTracker := ToolTracker{
		providerId: &providerID,
		cluster:    config.Cluster,
		client:     common.MCPDevelopmentClient,
	}
	elicitationsClient := NewMCPElicitationsController(s)
	ec := elicitations.NewController(elicitationsClient)
	elicitationsController = &ec
	defangTools := tools.CollectTools(ec, config)
	for i := range defangTools {
		defangTools[i].Handler = toolTracker.TrackTool(defangTools[i].Tool.Name, defangTools[i].Handler)
	}

	s.AddTools(defangTools...)
	return s, nil
}
