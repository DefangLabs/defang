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

// handleSetAWSProvider handles the set AWS provider MCP tool request
func handleSetAWSProvider(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string) (*mcp.CallToolResult, error) {
	term.Debug("Set AWS Provider tool called")
	var awsId string
	var awsSecretAccessKey string
	var awsRegion string
	var err error

	if common.ElicitationEnabled {
		mcpServer := server.ServerFromContext(ctx)
		sess := server.ClientSessionFromContext(ctx)
		if sess == nil {
			// no active session in this context
			return mcp.NewToolResultErrorFromErr("No active session", errors.New("no active session")), errors.New("no active session")
		}

		schema := map[string]any{
			"aws_id": map[string]any{
				"type":        "string",
				"title":       "AWS Access Key ID",
				"description": "Your AWS Access Key ID",
			},
			"aws_secret": map[string]any{
				"type":        "string",
				"title":       "AWS Secret Access Key",
				"description": "Your AWS Secret Access Key",
			},
			"aws_region": map[string]any{
				"type":        "string",
				"title":       "AWS Region",
				"description": "Your AWS Region",
			},
		}
		required := []string{"aws_id", "aws_secret", "aws_region"}
		request := common.NewElicitationRequest("Please provide your AWS information to configure deployment to your cloud", schema, required)
		result, err := mcpServer.RequestElicitation(ctx, request)
		if err != nil {
			term.Error("Failed to elicit AWS information", "error", err)
			return mcp.NewToolResultErrorFromErr("Failed to elicit AWS information", err), err
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
			return mcp.NewToolResultErrorFromErr("Invalid response format for AWS information", fmt.Errorf("expected map[string]any, got %T", result.Content)), errors.New("invalid response format")
		}
		awsId, ok = response["aws_id"].(string)
		if !ok {
			return mcp.NewToolResultErrorFromErr("Invalid AWS Access Key ID format", errors.New("expected string for aws_id")), errors.New("invalid aws_id format")
		}
		awsSecretAccessKey, ok = response["aws_secret"].(string)
		if !ok {
			return mcp.NewToolResultErrorFromErr("Invalid AWS Secret Access Key format", errors.New("expected string for aws_secret")), errors.New("invalid aws_secret format")
		}
		awsRegion, ok = response["aws_region"].(string)
		if !ok {
			return mcp.NewToolResultErrorFromErr("Invalid AWS Region format", errors.New("expected string for aws_region")), errors.New("invalid aws_region format")
		}
	} else {
		awsId, err = request.RequireString("aws_access_key_id")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Invalid AWS access key", err), err
		}
		awsSecretAccessKey, err = request.RequireString("aws_secret_access_key")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Invalid AWS secret access key", err), err
		}
		awsRegion, err = request.RequireString("aws_region")
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Invalid AWS region", err), err
		}
	}

	if err := actions.SetAWSByocProvider(ctx, cluster, providerId, awsId, awsSecretAccessKey, awsRegion); err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to set AWS provider", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully set the provider %q", *providerId)), nil
}
