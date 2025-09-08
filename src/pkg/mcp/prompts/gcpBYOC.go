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

func setupGCPBYOPrompt(s *server.MCPServer, cluster string, providerId *client.ProviderID) {
	gcpBYOPrompt := mcp.NewPrompt("GCP Setup",
		mcp.WithPromptDescription("Setup for GCP"),

		mcp.WithArgument("GCP_PROJECT_ID",
			mcp.ArgumentDescription("Your GCP Project ID"),
			mcp.RequiredArgument(),
		),
	)

	s.AddPrompt(gcpBYOPrompt, func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		projectID := getStringArg(req.Params.Arguments, "GCP_PROJECT_ID", "")

		if projectID == "" {
			return nil, errors.New("GCP_PROJECT_ID is required")
		}

		err := os.Setenv("GCP_PROJECT_ID", projectID)
		if err != nil {
			return nil, err
		}

		fabric, err := cli.Connect(ctx, cluster)
		if err != nil {
			return nil, err
		}

		_, err = tools.CheckProviderConfigured(ctx, fabric, client.ProviderGCP, "", 0)
		if err != nil {
			return nil, err
		}

		*providerId = client.ProviderGCP

		//FIXME: Should not be setting both the global var and env var
		err = os.Setenv("DEFANG_PROVIDER", "gcp")
		if err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "GCP BYOC Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleAssistant,
					Content: mcp.NewTextContent("Can you deploy my application now."),
				},
			},
		}, nil
	})
}
