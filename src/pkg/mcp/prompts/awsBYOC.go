package prompts

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupAwsByocPrompt(s *server.MCPServer, cluster string, providerId *client.ProviderID) {
	awsBYOCPrompt := mcp.NewPrompt("AWS Setup",
		mcp.WithPromptDescription("Setup for AWS"),

		mcp.WithArgument("AWS Credential",
			mcp.ArgumentDescription("Your AWS Access Key ID or AWS Profile Name"),
			mcp.RequiredArgument(),
		),

		mcp.WithArgument("AWS_SECRET_ACCESS_KEY",
			mcp.ArgumentDescription("Your AWS Secret Access Key"),
		),

		mcp.WithArgument("AWS_REGION",
			mcp.ArgumentDescription("Your AWS Region"),
		),
	)

	s.AddPrompt(awsBYOCPrompt, awsByocPromptHandler(cluster, providerId))
}

// awsByocPromptHandler is extracted for testability
func awsByocPromptHandler(cluster string, providerId *client.ProviderID) func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		awsID := req.Params.Arguments["AWS Credential"]
		region := common.GetStringArg(req.Params.Arguments, "AWS_REGION", "")
		awsSecret := common.GetStringArg(req.Params.Arguments, "AWS_SECRET_ACCESS_KEY", "")
		if err := actions.SetAWSByocProvider(ctx, providerId, cluster, awsID, awsSecret, region); err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "AWS BYOC Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent(common.PostPrompt),
				},
			},
		}, nil
	}
}
