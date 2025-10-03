package tools

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetGCPProvider handles the set GCP provider MCP tool request
func handleSetGCPProvider(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string) (*mcp.CallToolResult, error) {
	term.Debug("Set GCP Provider tool called")

	gcpProjectID, err := request.RequireString("gcpProjectId")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Invalid GCP project ID", err), err
	}

	if err := actions.SetGCPByocProvider(ctx, providerId, cluster, gcpProjectID); err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to set GCP provider", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully set the provider %q", *providerId)), nil
}
