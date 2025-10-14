package tools

import (
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/actions"
)

// HandleSetPlaygroundProvider handles the set Playground provider MCP tool request
func HandleSetPlaygroundProvider(providerId *cliClient.ProviderID) (string, error) {
	if err := actions.SetPlaygroundProvider(providerId); err != nil {
		return "", fmt.Errorf("Failed to set Playground provider: %w", err)
	}

	return fmt.Sprintf("Successfully set the provider %q", *providerId), nil
}
