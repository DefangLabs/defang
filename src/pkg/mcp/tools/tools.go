package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SetupTools configures and adds all the MCP tools to the server
func SetupTools(s *server.MCPServer, cluster string, authPort int, providerId *client.ProviderID) {
	// Create a tool for logging in and getting a new token
	term.Debug("Setting up login tool")
	setupLoginTool(s, cluster, authPort)

	// Create a tool for listing services
	term.Debug("Setting up services tool")
	setupServicesTool(s, cluster, providerId)

	// Create a tool for deployment
	term.Debug("Setting up deployment tool")
	setupDeployTool(s, cluster, providerId)

	// Create a tool for destroying services
	term.Debug("Setting up destroy tool")
	setupDestroyTool(s, cluster, providerId)

	// Create a tool for estimating costs
	term.Debug("Setting up estimate tool")
	setupEstimateTool(s, cluster, providerId)

	// Create a tool to set config variables
	term.Debug("Setting up set config tool")
	setupSetConfigTool(s, cluster, providerId)

	// Create a tool to remove config variables
	term.Debug("Setting up remove config tool")
	setupRemoveConfigTool(s, cluster, providerId)

	// Create a tool to list config variables
	term.Debug("Setting up list config tool")
	setupListConfigTool(s, cluster, providerId)

	// Add a tool that uses elicitation
	s.AddTool(
		mcp.NewTool(
			"create_project",
			mcp.WithDescription("Creates a new project with user-specified configuration"),
		),
		demoElicitationHandler(s),
	)

	// Add another tool that demonstrates conditional elicitation
	s.AddTool(
		mcp.NewTool(
			"process_data",
			mcp.WithDescription("Processes data with optional user confirmation"),
			mcp.WithString("data", mcp.Required(), mcp.Description("Data to process")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Safely extract data argument
			dataRaw, exists := request.GetArguments()["data"]
			if !exists {
				return nil, errors.New("required parameter 'data' is missing")
			}
			data, ok := dataRaw.(string)
			if !ok {
				return nil, fmt.Errorf("parameter 'data' must be a string, got %T", dataRaw)
			}

			// Only request elicitation if data seems sensitive
			if len(data) > 100 {
				elicitationRequest := mcp.ElicitationRequest{
					Params: mcp.ElicitationParams{
						Message: fmt.Sprintf("The data is %d characters long. Do you want to proceed with processing?", len(data)),
						RequestedSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"proceed": map[string]any{
									"type":        "boolean",
									"description": "Whether to proceed with processing",
								},
								"reason": map[string]any{
									"type":        "string",
									"description": "Optional reason for your decision",
								},
							},
							"required": []string{"proceed"},
						},
					},
				}

				result, err := s.RequestElicitation(ctx, elicitationRequest)
				if err != nil {
					return nil, fmt.Errorf("failed to get confirmation: %w", err)
				}

				if result.Action != mcp.ElicitationResponseActionAccept {
					return &mcp.CallToolResult{
						Content: []mcp.Content{
							mcp.NewTextContent("Processing cancelled by user"),
						},
					}, nil
				}

				// Safely extract response data
				responseData, ok := result.Content.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("unexpected response format: expected map[string]any, got %T", result.Content)
				}

				// Safely extract proceed field
				proceedRaw, exists := responseData["proceed"]
				if !exists {
					return nil, errors.New("required field 'proceed' is missing from response")
				}
				proceed, ok := proceedRaw.(bool)
				if !ok {
					return nil, fmt.Errorf("field 'proceed' must be a boolean, got %T", proceedRaw)
				}

				if !proceed {
					reason := "No reason provided"
					if reasonRaw, exists := responseData["reason"]; exists {
						if r, ok := reasonRaw.(string); ok && r != "" {
							reason = r
						} else if reasonRaw != nil {
							return nil, fmt.Errorf("field 'reason' must be a string, got %T", reasonRaw)
						}
					}
					return &mcp.CallToolResult{
						Content: []mcp.Content{
							mcp.NewTextContent("Processing declined: " + reason),
						},
					}, nil
				}
			}

			// Process the data
			processed := fmt.Sprintf("Processed %d characters of data", len(data))
			count := requestCount.Add(1)

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(fmt.Sprintf("%s (request #%d)", processed, count)),
				},
			}, nil
		},
	)

	term.Debug("All MCP tools have been set up successfully")
}
