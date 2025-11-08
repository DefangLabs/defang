package tools

import (
	"context"
	"errors"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
)

type SetGCPProviderParams struct {
	GCPProjectID string `json:"gcpProjectId"`
}

// HandleSetGCPProvider handles the set GCP provider MCP tool request
func HandleSetGCPProvider(ctx context.Context, params SetGCPProviderParams, providerId *cliClient.ProviderID, cluster string) (string, error) {
	if params.GCPProjectID == "" {
		return "", errors.New("GCP project ID cannot be empty")
	}

	if err := actions.SetGCPByocProvider(ctx, providerId, cluster, params.GCPProjectID); err != nil {
		return "", fmt.Errorf("Failed to set GCP provider: %w", err)
	}

	return fmt.Sprintf("Successfully set the provider %q", *providerId), nil
}
