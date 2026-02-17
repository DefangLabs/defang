package mcp

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/elicitations"
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

func (c *mcpElicitationsController) Request(ctx context.Context, req elicitations.Request) (elicitations.Response, error) {
	result, err := c.server.RequestElicitation(ctx, mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message:         req.Message,
			RequestedSchema: req.Schema,
		},
	})
	if err != nil {
		return elicitations.Response{}, err
	}

	// cast result.Content to map[string]any
	contentMap, ok := result.Content.(map[string]any)
	if !ok {
		return elicitations.Response{}, fmt.Errorf("invalid eliciation response content type, got %T", result.Content)
	}

	// TODO: right now you can only pass a single validator even though the
	// schema can have multiple fields. This is fine for now, because we only
	// ever have one field in our schemas, but we should eventually support
	// multiple validators for multiple fields.
	if req.Validator != nil {
		if props, ok := req.Schema["properties"].(map[string]any); ok {
			for field := range props {
				if value, exists := contentMap[field]; exists {
					if err := req.Validator(value); err != nil {
						return elicitations.Response{}, fmt.Errorf("validation failed for field '%s': %w", field, err)
					}
				}
			}
		}
	}

	return elicitations.Response{
		Action:  string(result.Action),
		Content: contentMap,
	}, nil
}
