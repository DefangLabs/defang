package tools

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// demoElicitationHandler demonstrates how to use elicitation in a tool
func demoElicitationHandler(s *server.MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Create an elicitation request to get project details
		elicitationRequest := mcp.ElicitationRequest{
			Params: mcp.ElicitationParams{
				Message: "I need some information to set up your project. Please provide the project details.",
				RequestedSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"projectName": map[string]any{
							"type":        "string",
							"description": "Name of the project",
							"minLength":   1,
						},
						"framework": map[string]any{
							"type":        "string",
							"description": "Frontend framework to use",
							"enum":        []string{"react", "vue", "angular", "none"},
						},
						"includeTests": map[string]any{
							"type":        "boolean",
							"description": "Include test setup",
							"default":     true,
						},
					},
					"required": []string{"projectName"},
				},
			},
		}

		// Request elicitation from the client
		result, err := s.RequestElicitation(ctx, elicitationRequest)
		if err != nil {
			return nil, fmt.Errorf("failed to request elicitation: %w", err)
		}

		// Handle the user's response
		switch result.Action {
		case mcp.ElicitationResponseActionAccept:
			// User provided the information
			data, ok := result.Content.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("unexpected response format: expected map[string]any, got %T", result.Content)
			}

			// Safely extract projectName (required field)
			projectNameRaw, exists := data["projectName"]
			if !exists {
				return nil, errors.New("required field 'projectName' is missing from response")
			}
			projectName, ok := projectNameRaw.(string)
			if !ok {
				return nil, fmt.Errorf("field 'projectName' must be a string, got %T", projectNameRaw)
			}
			if projectName == "" {
				return nil, errors.New("field 'projectName' cannot be empty")
			}

			// Safely extract framework (optional field)
			framework := "none"
			if frameworkRaw, exists := data["framework"]; exists {
				if fw, ok := frameworkRaw.(string); ok {
					framework = fw
				} else {
					return nil, fmt.Errorf("field 'framework' must be a string, got %T", frameworkRaw)
				}
			}

			// Safely extract includeTests (optional field)
			includeTests := true
			if testsRaw, exists := data["includeTests"]; exists {
				if tests, ok := testsRaw.(bool); ok {
					includeTests = tests
				} else {
					return nil, fmt.Errorf("field 'includeTests' must be a boolean, got %T", testsRaw)
				}
			}

			// Create project based on user input
			message := fmt.Sprintf(
				"Created project '%s' with framework: %s, tests: %v",
				projectName, framework, includeTests,
			)

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(message),
				},
			}, nil

		case mcp.ElicitationResponseActionDecline:
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent("Project creation cancelled - user declined to provide information"),
				},
			}, nil

		case mcp.ElicitationResponseActionCancel:
			return nil, errors.New("project creation cancelled by user")

		default:
			return nil, fmt.Errorf("unexpected response action: %s", result.Action)
		}
	}
}

var requestCount atomic.Int32
