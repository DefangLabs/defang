package tools

import (
	"context"
	"errors"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// handleSetGCPProvider handles the set GCP provider MCP tool request
func handleSetGCPProvider(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string) (*mcp.CallToolResult, error) {
	term.Debug("Set GCP Provider tool called")
	var gcpProjectID string
	var err error

	if common.ElicitationEnabled {
		mcpServer := server.ServerFromContext(ctx)
		sess := server.ClientSessionFromContext(ctx)
		if sess == nil {
			// no active session in this context
			return mcp.NewToolResultErrorFromErr("No active session", errors.New("no active session")), errors.New("no active session")
		}

		schema := map[string]any{
			"gcp_project_id": map[string]any{
				"type":        "string",
				"title":       "GCP Project ID",
				"description": "Your GCP Project ID",
			},
		}
		required := []string{"gcp_project_id"}
		request := common.NewElicitationRequest("Please provide your GCP information to configure deployment to your cloud", schema, required)
		result, err := mcpServer.RequestElicitation(ctx, request)
		if err != nil {
			term.Error("Failed to elicit GCP information", "error", err)
			return mcp.NewToolResultErrorFromErr("Failed to elicit GCP information", err), err
		}

		if result.Action != mcp.ElicitationResponseActionAccept {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent("Processing cancelled by user"),
				},
			}, nil
		}

		response, ok := result.Content.(map[string]any)
		if !ok {
			return mcp.NewToolResultErrorFromErr("Invalid response format for GCP information", fmt.Errorf("expected map[string]any, got %T", result.Content)), errors.New("invalid response format")
		}
		gcpProjectID, ok = response["gcp_project_id"].(string)
		if !ok {
			return mcp.NewToolResultErrorFromErr("Invalid GCP Project ID format", errors.New("expected string for gcp_project_id")), errors.New("invalid gcp_project_id format")
		}
	} else {
		gcpProjectID, err = request.RequireString("gcp_project_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Invalid GCP project ID", err), err
		}
	}

	if err := actions.SetGCPByocProvider(ctx, providerId, cluster, gcpProjectID); err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to set GCP provider", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully set the provider %q", *providerId)), nil
}
