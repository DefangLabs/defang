package tools

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type RemoveConfigParams struct {
	common.LoaderParams
	Name string `json:"name" jsonschema:"required"`
}

// HandleRemoveConfigTool handles the remove config tool logic
func HandleRemoveConfigTool(ctx context.Context, params RemoveConfigParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	_, provider, loader, err := setupProviderAndLoader(ctx, params.LoaderParams, cli, ec, sc)
	if err != nil {
		return setupErrorResult(err)
	}
	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}
	if err := cli.ConfigDelete(ctx, projectName, provider, params.Name); err != nil {
		// Show a warning (not an error) if the config was not found
		if connect.CodeOf(err) == connect.CodeNotFound {
			return fmt.Sprintf("Config variable %q not found in project %q in stack %q", params.Name, projectName, sc.Stack.Name), nil
		}
		return "", fmt.Errorf("failed to remove config variable %q from project %q in stack %q: %w", params.Name, projectName, sc.Stack.Name, err)
	}

	return fmt.Sprintf("Successfully removed the config variable %q from project %q in stack %q", params.Name, projectName, sc.Stack.Name), nil
}
