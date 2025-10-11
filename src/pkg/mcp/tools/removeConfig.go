package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
)

type RemoveConfigParams struct {
	Name string
}

func parseRemoveConfigParams(request mcp.CallToolRequest) (RemoveConfigParams, error) {
	name, err := request.RequireString("name")
	if err != nil || name == "" {
		return RemoveConfigParams{}, fmt.Errorf("missing config `name`: %w", err)
	}
	return RemoveConfigParams{
		Name: name,
	}, nil
}

// handleRemoveConfigTool handles the remove config tool logic
func handleRemoveConfigTool(ctx context.Context, loader cliClient.ProjectLoader, params RemoveConfigParams, providerId *cliClient.ProviderID, fabric client.FabricClient, cli RemoveConfigCLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", fmt.Errorf("No provider configured: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider, err := cli.NewProvider(ctx, *providerId, fabric)
	if err != nil {
		return "", fmt.Errorf("Failed to get new provider: %w", err)
	}

	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	term.Debug("Function invoked: cli.ConfigDelete")
	if err := cli.ConfigDelete(ctx, projectName, provider, params.Name); err != nil {
		// Show a warning (not an error) if the config was not found
		if connect.CodeOf(err) == connect.CodeNotFound {
			return fmt.Sprintf("Config variable %q not found in project %q", params.Name, projectName), nil
		}
		return "", fmt.Errorf("Failed to remove config variable %q from project %q: %w", params.Name, projectName, err)
	}

	return fmt.Sprintf("Successfully remove the config variable %q from project %q", params.Name, projectName), nil
}
