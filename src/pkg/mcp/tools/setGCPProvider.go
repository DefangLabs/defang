package tools

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetGCPProvider handles the set GCP provider MCP tool request
func handleSetGCPProvider(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string) (string, error) {
	gcpProjectID, err := request.RequireString("gcpProjectId")
	if err != nil {
		return "", fmt.Errorf("Invalid GCP project ID: %w", err)
	}

	if err := actions.SetGCPByocProvider(ctx, providerId, cluster, gcpProjectID); err != nil {
		return "", fmt.Errorf("Failed to set GCP provider: %w", err)
	}

	return fmt.Sprintf("Successfully set the provider %q", *providerId), nil
}
