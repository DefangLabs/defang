package tools

import (
	"context"
	"fmt"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleListConfigTool handles the list config tool logic
func handleListConfigTool(ctx context.Context, loader cliClient.ProjectLoader, _ mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli ListConfigCLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", fmt.Errorf("No provider configured: %w", err)
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

	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	term.Debug("Function invoked: cli.ConfigList")
	config, err := cli.ListConfig(ctx, provider, projectName)
	if err != nil {
		return "", fmt.Errorf("Failed to list config variables: %w", err)
	}

	numConfigs := len(config.Names)
	if numConfigs == 0 {
		return fmt.Sprintf("No config variables found for the project %q.", projectName), nil
	}

	configNames := make([]string, numConfigs)
	copy(configNames, config.Names)

	return fmt.Sprintf("Here is the list of config variables for the project %q: %v", projectName, strings.Join(configNames, ", ")), nil
}
