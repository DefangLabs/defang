package prompts

import (
	"context"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupGcpByocPrompt(s *server.MCPServer, cluster string, providerId *client.ProviderID) {
	gcpBYOCPrompt := mcp.NewPrompt("GCP Setup",
		mcp.WithPromptDescription("Setup for GCP"),
		mcp.WithArgument("GCP_PROJECT_ID",
			mcp.ArgumentDescription("Your GCP Project ID"),
			mcp.RequiredArgument(),
		),
	)

	s.AddPrompt(gcpBYOCPrompt, gcpByocPromptHandler(cluster, providerId))
}

// gcpByocPromptHandler is extracted for testability
func gcpByocPromptHandler(cluster string, providerId *client.ProviderID) func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		// Can never be nil or empty due to RequiredArgument
		projectID := req.Params.Arguments["GCP_PROJECT_ID"]

		err := os.Setenv("GCP_PROJECT_ID", projectID)
		if err != nil {
			return nil, err
		}

		fabric, err := common.Connect(ctx, cluster)
		if err != nil {
			return nil, err
		}

		_, err = common.CheckProviderConfigured(ctx, fabric, client.ProviderGCP, "", 0)
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
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent(postPrompt),
				},
			},
		}, nil
	}
}
