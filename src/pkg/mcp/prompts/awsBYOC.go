package prompts

import (
	"context"
	"errors"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupAWSBYOPrompt(s *server.MCPServer, cluster string, providerId *client.ProviderID) {
	awsBYOCPrompt := mcp.NewPrompt("AWS Setup",
		mcp.WithPromptDescription("Setup for AWS"),

		mcp.WithArgument("AWS_Credential",
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

	s.AddPrompt(awsBYOCPrompt, func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		// Can never be nil or empty due to RequiredArgument
		awsID := req.Params.Arguments["AWS_Credential"]
		if isValidAWSKey(awsID) {
			err := os.Setenv("AWS_ACCESS_KEY_ID", awsID)
			if err != nil {
				return nil, err
			}

			awsSecret := getStringArg(req.Params.Arguments, "AWS_SECRET_ACCESS_KEY", "")
			region := getStringArg(req.Params.Arguments, "AWS_REGION", "")

			if awsSecret == "" {
				return nil, errors.New("AWS_SECRET_ACCESS_KEY is required")
			}

			err = os.Setenv("AWS_SECRET_ACCESS_KEY", awsSecret)
			if err != nil {
				return nil, err
			}

			if region == "" {
				return nil, errors.New("AWS_REGION is required")
			}

			err = os.Setenv("AWS_REGION", region)
			if err != nil {
				return nil, err
			}
		} else {
			err := os.Setenv("AWS_PROFILE", awsID)
			if err != nil {
				return nil, err
			}
		}

		fabric, err := cli.Connect(ctx, cluster)
		if err != nil {
			return nil, err
		}

		_, err = tools.CheckProviderConfigured(ctx, fabric, client.ProviderAWS, "", 0)
		if err != nil {
			return nil, err
		}

		*providerId = client.ProviderAWS

		//FIXME: Should not be setting both the global and env var
		err = os.Setenv("DEFANG_PROVIDER", "aws")
		if err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "AWS BYOC Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("Can you deploy my application now."),
				},
			},
		}, nil
	})
}

// Check if the provided AWS access key ID is valid
func isValidAWSKey(key string) bool {
	// Define accepted AWS access key prefixes
	acceptedPrefixes := map[string]bool{
		"ABIA": true,
		"ACCA": true,
		"AGPA": true,
		"AIDA": true,
		"AKPA": true,
		"AKIA": true,
		"ANPA": true,
		"ANVA": true,
		"APKA": true,
		"AROA": true,
		"ASCA": true,
		"ASIA": true,
	}

	if len(key) < 16 {
		return false
	}

	prefix := key[:4]
	_, ok := acceptedPrefixes[prefix]
	if !ok {
		return false
	}

	return true
}
