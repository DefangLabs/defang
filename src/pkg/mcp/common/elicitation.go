package common

import "github.com/mark3labs/mcp-go/mcp"

func NewElicitationRequest(
	message string,
	schema map[string]any,
	requiredKeys []string,
) mcp.ElicitationRequest {
	return mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: message,
			RequestedSchema: map[string]any{
				"type":       "object",
				"properties": schema,
				"required":   requiredKeys,
			},
		},
	}
}
