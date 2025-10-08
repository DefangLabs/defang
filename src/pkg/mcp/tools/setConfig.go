package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetConfig handles the set config MCP tool request
func handleSetConfig(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli SetConfigCLIInterface) (string, error) {
	term.Debug("Set Config tool called")

	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", fmt.Errorf("No provider configured: %w", err)
	}

	wd, err := request.RequireString("working_directory")
	if err != nil || wd == "" {
		return "", fmt.Errorf("Invalid working directory: %w", errors.New("working_directory is required"))
	}

	err = os.Chdir(wd)
	if err != nil {
		return "", fmt.Errorf("Failed to change working directory: %w", err)
	}

	name, err := request.RequireString("name")
	if err != nil || name == "" {
		return "", fmt.Errorf("Invalid config `name`: %w", errors.New("`name` is required"))
	}

	value, err := request.RequireString("value")
	if err != nil || value == "" {
		return "", fmt.Errorf("Invalid config `value`: %w", errors.New("`value` is required"))
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

	loader := common.ConfigureLoader(request)

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	if !pkg.IsValidSecretName(name) {
		return "", fmt.Errorf("Invalid secret name: secret name %q is not valid", name)
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cli.ConfigSet(ctx, projectName, provider, name, value); err != nil {
		return "", fmt.Errorf("Failed to set config: %w", err)
	}

	return fmt.Sprintf("Successfully set the config variable %q for project %q", name, projectName), nil
}
