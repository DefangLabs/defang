package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type ListConfigsParams struct {
	common.LoaderParams
}

// HandleListConfigTool handles the list config tool logic
func HandleListConfigTool(ctx context.Context, loader cliClient.ProjectLoader, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, sc.Cluster)
	if err != nil {
		return "", fmt.Errorf("Could not connect: %w", err)
	}

	projectName, err := cli.LoadProjectName(ctx, loader)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)
	pp := NewProviderPreparer(cli, ec, client)
	_, provider, err := pp.SetupProvider(ctx, projectName, sc.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	term.Debug("Function invoked: cli.ConfigList")
	config, err := cli.ListConfig(ctx, provider, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to list config variables: %w", err)
	}

	numConfigs := len(config.Names)
	if numConfigs == 0 {
		return fmt.Sprintf("No config variables found for the project %q.", projectName), nil
	}

	configNames := make([]string, numConfigs)
	copy(configNames, config.Names)

	return fmt.Sprintf("Here is the list of config variables for the project %q: %v", projectName, strings.Join(configNames, ", ")), nil
}
