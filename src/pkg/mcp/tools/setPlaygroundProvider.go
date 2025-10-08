package tools

import (
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// handleSetPlaygroundProvider handles the set Playground provider MCP tool request
func handleSetPlaygroundProvider(providerId *cliClient.ProviderID) (string, error) {
	term.Debug("Set Playground Provider tool called")
	if err := actions.SetPlaygroundProvider(providerId); err != nil {
		return "", fmt.Errorf("Failed to set Playground provider: %w", err)
	}

	return fmt.Sprintf("Successfully set the provider %q", *providerId), nil
}
