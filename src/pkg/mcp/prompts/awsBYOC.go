package prompts

import (
	"context"
	"errors"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupAWSBYOPrompt(s *server.MCPServer, providerId *client.ProviderID) {
	awsBYOCPrompt := mcp.NewPrompt("AWS BYOC Setup",
		mcp.WithPromptDescription("Bring Your Own Cloud setup for AWS"),

		mcp.WithArgument("AWS_ACCESS_KEY_ID",
			mcp.ArgumentDescription("Your AWS Access Key ID"),
			mcp.RequiredArgument(),
		),

		mcp.WithArgument("AWS_SECRET_ACCESS_KEY",
			mcp.ArgumentDescription("Your AWS Secret Access Key"),
			mcp.RequiredArgument(),
		),
	)

	s.AddPrompt(awsBYOCPrompt, func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		awsID := getStringArg(req.Params.Arguments, "AWS_ACCESS_KEY_ID", "")
		awsSecret := getStringArg(req.Params.Arguments, "AWS_SECRET_ACCESS_KEY", "")

		providerId.Set(client.ProviderAWS.String())

		err := os.Setenv("DEFANG_PROVIDER", "aws")
		if err != nil {
			return nil, err
		}

		if awsID == "" {
			return nil, errors.New("AWS_ACCESS_KEY_ID is required")
		}

		err = os.Setenv("AWS_ACCESS_KEY_ID", awsID)
		if err != nil {
			return nil, err
		}

		if awsSecret == "" {
			return nil, errors.New("AWS_SECRET_ACCESS_KEY is required")
		}

		err = os.Setenv("AWS_SECRET_ACCESS_KEY", awsSecret)
		if err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "AWS BYOC Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("Deploy this application to Defang."),
				},
			},
		}, nil
	})
}
