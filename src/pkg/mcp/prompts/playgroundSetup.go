package prompts

import (
	"context"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func setupPlaygroundPrompt(s *server.MCPServer, providerId *client.ProviderID) {
	playgroundPrompt := mcp.NewPrompt("Playground Setup",
		mcp.WithPromptDescription("Setup for Playground"),
	)

	s.AddPrompt(playgroundPrompt, func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		*providerId = client.ProviderDefang

		//FIXME: Should not be setting both the global and env var
		err := os.Setenv("DEFANG_PROVIDER", "defang")
		if err != nil {
			return nil, err
		}

		return &mcp.GetPromptResult{
			Description: "Defang playground Setup Complete",
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent(postPrompt),
				},
			},
		}, nil
	})
}
