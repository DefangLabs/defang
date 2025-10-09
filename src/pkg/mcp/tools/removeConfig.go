package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleRemoveConfigTool handles the remove config tool logic
func handleRemoveConfigTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli RemoveConfigCLIInterface) (string, error) {
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
		return "", fmt.Errorf("Invalid config `name`: %w", err)
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

	loader := cli.ConfigureLoader(request)

	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	term.Debug("Function invoked: cli.ConfigDelete")
	if err := cli.ConfigDelete(ctx, projectName, provider, name); err != nil {
		// Show a warning (not an error) if the config was not found
		if connect.CodeOf(err) == connect.CodeNotFound {
			return fmt.Sprintf("Config variable %q not found in project %q", name, projectName), nil
		}
		return "", fmt.Errorf("Failed to remove config variable %q from project %q: %w", name, projectName, err)
	}

	return fmt.Sprintf("Successfully remove the config variable %q from project %q", name, projectName), nil
}
