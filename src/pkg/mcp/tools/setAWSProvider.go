package tools

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetAWSProvider handles the set AWS provider MCP tool request
func handleSetAWSProvider(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string) (*mcp.CallToolResult, error) {
	term.Debug("Set AWS Provider tool called")
	awsId, err := request.RequireString("aws_access_key_id")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Invalid AWS access key", err), err
	}
	awsSecretAccessKey, err := request.RequireString("aws_secret_access_key")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Invalid AWS secret access key", err), err
	}
	awsRegion, err := request.RequireString("aws_region")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Invalid AWS region", err), err
	}

	if err := actions.SetAWSByocProvider(ctx, cluster, providerId, awsId, awsSecretAccessKey, awsRegion); err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to set AWS provider", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully set the provider %q", *providerId)), nil
}
