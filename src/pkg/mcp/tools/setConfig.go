package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetConfig handles the set config MCP tool request
func handleSetConfig(ctx context.Context, loader cliClient.ProjectLoader, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli SetConfigCLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", err
	}

	name, err := request.RequireString("name")
	if err != nil || name == "" {
		return "", fmt.Errorf("Invalid config `name`: %w", err)
	}

	value, err := request.RequireString("value")
	if err != nil || value == "" {
		return "", fmt.Errorf("Invalid config `value`: %w", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("Could not connect: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider, err := cli.NewProvider(ctx, *providerId, client)
	if err != nil {
		return "", fmt.Errorf("Failed to get new provider: %w", err)
	}

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	if !pkg.IsValidSecretName(name) {
		return "", fmt.Errorf("Invalid config name: secret name %q is not valid", name)
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cli.ConfigSet(ctx, projectName, provider, name, value); err != nil {
		return "", fmt.Errorf("Failed to set config: %w", err)
	}

	return fmt.Sprintf("Successfully set the config variable %q for project %q", name, projectName), nil
}
