package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type ListConfigParams struct {
	common.LoaderParams
}

// HandleListConfigTool handles the list config tool logic
func HandleListConfigTool(ctx context.Context, loader client.Loader, params ListConfigParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := GetClientWithRetry(ctx, cli, sc)
	if err != nil {
		var noBrowserErr auth.ErrNoBrowser
		if errors.As(err, &noBrowserErr) {
			return noBrowserErr.Error(), nil
		}
		return "", err
	}

	sm, err := stacks.NewManager(client, loader.TargetDirectory(ctx), params.ProjectName, ec)
	if err != nil {
		return "", fmt.Errorf("failed to create stack manager: %w", err)
	}
	pp := NewProviderPreparer(cli, ec, client, sm)
	_, provider, err := pp.SetupProvider(ctx, sc.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	term.Debug("Function invoked: cli.LoadProjectName")
	projectName, err := cli.LoadProjectName(ctx, loader, provider)
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
		return fmt.Sprintf("No config variables found for the project %q.", projectName), nil
	}

	configNames := make([]string, numConfigs)
	copy(configNames, config.Names)

	return fmt.Sprintf("Here is the list of config variables for the project %q: %v", projectName, strings.Join(configNames, ", ")), nil
}
