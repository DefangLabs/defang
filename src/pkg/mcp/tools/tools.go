package tools

import (
	"context"

	agentTools "github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/firebase/genkit/go/ai"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type StackConfig struct {
	Cluster    string
	ProviderID *client.ProviderID
	Stack      string
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

func CollectTools(ec elicitations.Controller, config StackConfig) []server.ServerTool {
	genkitTools := agentTools.CollectDefangTools(ec, agentTools.StackConfig{
		Cluster:    config.Cluster,
		ProviderID: config.ProviderID,
		Stack:      config.Stack,
	})
	return translateGenKitToolsToMCP(genkitTools)
}
