package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

type SetConfigParams struct {
	common.LoaderParams
	Name  string
	Value string
}

func ParseSetConfigParams(request mcp.CallToolRequest) (SetConfigParams, error) {
	name, err := request.RequireString("name")
	if err != nil || name == "" {
		return SetConfigParams{}, fmt.Errorf("missing 'name' parameter: %w", err)
	}
	value, err := request.RequireString("value")
	if err != nil || value == "" {
		return SetConfigParams{}, fmt.Errorf("missing 'value' parameter: %w", err)
	}
	return SetConfigParams{
		Name:  name,
		Value: value,
	}, nil
}

// HandleSetConfig handles the set config MCP tool request
func HandleSetConfig(ctx context.Context, loader cliClient.ProjectLoader, params SetConfigParams, providerId *cliClient.ProviderID, cluster string, cli SetConfigCLIInterface) (string, error) {
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

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	if !pkg.IsValidSecretName(params.Name) {
		return "", fmt.Errorf("Invalid config name: secret name %q is not valid", params.Name)
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cli.ConfigSet(ctx, projectName, provider, params.Name, params.Value); err != nil {
		return "", fmt.Errorf("Failed to set config: %w", err)
	}

	return fmt.Sprintf("Successfully set the config variable %q for project %q", params.Name, projectName), nil
}
