package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

type RemoveConfigParams struct {
	common.LoaderParams
	Name string `json:"name" jsonschema:"required"`
}

// HandleRemoveConfigTool handles the remove config tool logic
func HandleRemoveConfigTool(ctx context.Context, loader cliClient.ProjectLoader, params RemoveConfigParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, sc.Cluster)
	if err != nil {
		return "", fmt.Errorf("Could not connect: %w", err)
	}

	pp := NewProviderPreparer(cli, ec, client)
	_, provider, err := pp.SetupProvider(ctx, &sc.Stack.Name)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}
	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}
	if err := cli.ConfigDelete(ctx, projectName, provider, params.Name); err != nil {
		// Show a warning (not an error) if the config was not found
		if connect.CodeOf(err) == connect.CodeNotFound {
			return fmt.Sprintf("Config variable %q not found in project %q", params.Name, projectName), nil
		}
		return "", fmt.Errorf("failed to remove config variable %q from project %q: %w", params.Name, projectName, err)
	}

	return fmt.Sprintf("Successfully removed the config variable %q from project %q", params.Name, projectName), nil
}
