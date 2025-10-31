package prompts

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
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
		projectID := req.Params.Arguments["GCP_PROJECT_ID"]
		if err := actions.SetGCPByocProvider(ctx, providerId, cluster, projectID); err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "GCP BYOC Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent(common.PostPrompt),
				},
			},
		}, nil
	}
}
