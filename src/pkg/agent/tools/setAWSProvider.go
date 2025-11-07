package tools

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleSetAWSProvider handles the set AWS provider MCP tool request
func HandleSetAWSProvider(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string) (string, error) {
	awsId, err := request.RequireString("accessKeyId")
	if err != nil {
		return "", fmt.Errorf("Invalid AWS access key Id: %w", err)
	}
	awsSecretAccessKey, err := request.RequireString("secretAccessKey")
	if err != nil {
		return "", fmt.Errorf("Invalid AWS secret access key: %w", err)
	}
	awsRegion, err := request.RequireString("region")
	if err != nil {
		return "", fmt.Errorf("Invalid AWS region: %w", err)
	}
	if err := actions.SetAWSByocProvider(ctx, providerId, cluster, awsId, awsSecretAccessKey, awsRegion); err != nil {
		return "", fmt.Errorf("Failed to set AWS provider: %w", err)
	}

	return fmt.Sprintf("Successfully set the provider %q", *providerId), nil
}
