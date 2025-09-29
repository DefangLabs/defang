package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/DefangLabs/defang/src/pkg/mcp/prompts"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
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

type ToolTracker struct {
	providerId string
	cluster    string
	client     string
}

func (t *ToolTracker) TrackTool(name string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := request.Params.Name
		track.Evt("MCP Tool Called", track.P("tool", name), track.P("client", t.client), track.P("cluster", t.cluster), track.P("provider", t.providerId))
		resp, err := handler(ctx, request)
		track.Evt("MCP Tool Done", track.P("tool", name), track.P("client", t.client), track.P("cluster", t.cluster), track.P("provider", t.providerId), track.P("error", err))
		return resp, err
	}
}

func OnRequestInitializationHandler(ctx context.Context, id any, message any) error {
	log.Print("OnRequestInitializationHandler is called:\n")

	rawMsg, ok := message.(json.RawMessage)
	if !ok {
		return errors.New("Init Req: invalid message type")
	}

	var initReq mcp.InitializeRequest
	if err := json.Unmarshal(rawMsg, &initReq); err != nil {
		return errors.New("Init Req: failed to unmarshal message")
	}

	// Pretty print capabilities
	if data, err := json.MarshalIndent(initReq.Params.Capabilities, "", "  "); err == nil {
		log.Print("Client Capabilities:\n" + string(data))
	}

	log.Printf("server config - %v:", initReq)

	log.Print("OnRequestInitializationHandler is done")

	return nil
}

func NewDefangMCPServer(version string, cluster string, authPort int, providerID *cliClient.ProviderID, client MCPClient) (*server.MCPServer, error) {
	// Setup knowledge base
	if err := SetupKnowledgeBase(); err != nil {
		return nil, fmt.Errorf("failed to setup knowledge base: %w", err)
	}

	defangTools := tools.CollectTools(cluster, authPort, providerID)
	s := server.NewMCPServer(
		"Deploy with Defang",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithToolCapabilities(true),
		server.WithInstructions(prepareInstructions(defangTools)),
	)

	resources.SetupResources(s)
	prompts.SetupPrompts(s, cluster, providerID)

	toolTracker := ToolTracker{
		providerId: string(*providerID),
		cluster:    cluster,
		client:     tools.MCPDevelopmentClient,
	}
	for i := range defangTools {
		defangTools[i].Handler = toolTracker.TrackTool(defangTools[i].Tool.Name, defangTools[i].Handler)
	}

	s.AddTools(defangTools...)
	return s, nil
}
