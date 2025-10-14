package prompts

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func playgroundPromptHandler(providerId *client.ProviderID) func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		if err := actions.SetPlaygroundProvider(providerId); err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "Defang playground Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent(common.PostPrompt),
				},
			},
		}, nil
	}
}

func setupPlaygroundPrompt(s *server.MCPServer, providerId *client.ProviderID) {
	playgroundPrompt := mcp.NewPrompt("Playground Setup",
		mcp.WithPromptDescription("Setup for Playground"),
	)

	s.AddPrompt(playgroundPrompt, playgroundPromptHandler(providerId))
}
