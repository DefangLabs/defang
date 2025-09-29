package tools

import (
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetPlaygroundProvider handles the set Playground provider MCP tool request
func handleSetPlaygroundProvider(providerId *cliClient.ProviderID) (*mcp.CallToolResult, error) {
	term.Debug("Set Playground Provider tool called")
	if err := actions.SetPlaygroundProvider(providerId); err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to set Playground provider", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully set the provider %q", *providerId)), nil
}
