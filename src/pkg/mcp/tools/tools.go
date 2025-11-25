package tools

import (
	"context"
	"errors"

	agentTools "github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type mcpElicitationsController struct {
	server *server.MCPServer
}

func NewMCPElicitationsController(server *server.MCPServer) *mcpElicitationsController {
	return &mcpElicitationsController{
		server: server,
	}
}

func (c *mcpElicitationsController) Request(ctx context.Context, req agentTools.ElicitationRequest) (agentTools.ElicitationResponse, error) {
	result, err := c.server.RequestElicitation(ctx, mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message:         req.Message,
			RequestedSchema: req.Schema,
		},
	})
	if err != nil {
		return agentTools.ElicitationResponse{}, err
	}

	// cast result.Content to map[string]string
	contentMap, ok := result.Content.(map[string]string)
	if !ok {
		return agentTools.ElicitationResponse{}, errors.New("invalid elicitation response content")
	}

	return agentTools.ElicitationResponse{
		Action:  string(result.Action),
		Content: contentMap,
	}, nil
}

func translateSchema(schema map[string]any) mcp.ToolInputSchema {
	if schema == nil {
		return mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		}
	}

	schemaType, ok := schema["type"].(string)
	if !ok {
		schemaType = "object"
	}
	schemaProperties, ok := schema["properties"].(map[string]any)
	if !ok {
		schemaProperties = map[string]any{}
	}
	schemaRequired, ok := schema["required"].([]string)
	if !ok {
		schemaRequired = []string{}
	}

	return mcp.ToolInputSchema{
		Type:       schemaType,
		Properties: schemaProperties,
		Required:   schemaRequired,
	}
}

func translateGenKitToolsToMCP(genkitTools []ai.Tool) []server.ServerTool {
	var translatedTools []server.ServerTool
	for _, t := range genkitTools {
		def := t.Definition()
		inputSchema := translateSchema(def.InputSchema)
		translatedTools = append(translatedTools, server.ServerTool{
			Tool: mcp.Tool{
				Name:        t.Name(),
				Description: def.Description,
				InputSchema: inputSchema,
			},
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result, err := t.RunRaw(ctx, request.GetArguments())
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Tool execution failed", err), nil
				}
				output, ok := result.(string)
				if !ok {
					return mcp.NewToolResultError("Tool returned unexpected result type"), nil
				}
				return mcp.NewToolResultText(output), nil
			},
		})
	}

	return translatedTools
}

func CollectTools(server *server.MCPServer, cluster string, providerId *client.ProviderID) []server.ServerTool {
	ec := NewMCPElicitationsController(server)
	genkitTools := agentTools.CollectDefangTools(cluster, ec, providerId)
	return translateGenKitToolsToMCP(genkitTools)
}
