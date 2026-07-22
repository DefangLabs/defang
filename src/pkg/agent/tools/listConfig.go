package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type ListConfigParams struct {
	common.LoaderParams
}

// HandleListConfigTool handles the list config tool logic
func HandleListConfigTool(ctx context.Context, params ListConfigParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	_, provider, loader, err := setupProviderAndLoader(ctx, params.LoaderParams, cli, ec, sc)
	if err != nil {
		return setupErrorResult(err)
	}

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	term.Debug("Function invoked: cli.ConfigList")
	config, err := cli.ListConfig(ctx, provider, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to list config variables: %w", err)
	}

	numConfigs := len(config.Names)
	if numConfigs == 0 {
		return fmt.Sprintf("No config variables found for the project %q in stack %q.", projectName, sc.Stack.Name), nil
	}

	configNames := make([]string, numConfigs)
	copy(configNames, config.Names)

	return fmt.Sprintf("Here is the list of config variables for the project %q in stack %q: %v", projectName, sc.Stack.Name, strings.Join(configNames, ", ")), nil
}
