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

	// cast result.Content to map[string]string
	contentMap, ok := result.Content.(map[string]any)
	if !ok {
		return elicitations.Response{}, fmt.Errorf("invalid eliciation response content type, got %T", result.Content)
	}

	return elicitations.Response{
		Action:  string(result.Action),
		Content: contentMap,
	}, nil
}
