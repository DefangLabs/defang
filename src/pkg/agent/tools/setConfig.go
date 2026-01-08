package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type SetConfigParams struct {
	common.LoaderParams
	Name   string `json:"name" jsonschema:"required"`
	Value  string `json:"value"`
	Random bool   `json:"random,omitempty" jsonschema:"description=Generate a secure randomly generated value for config (default: false)"`
}

func HandleSetConfig(ctx context.Context, loader client.Loader, params SetConfigParams, cliInterface CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := cliInterface.Connect(ctx, sc.Cluster)
	if err != nil {
		return "", fmt.Errorf("Could not connect: %w", err)
	}

	sm, err := stacks.NewManager(client, loader.TargetDirectory(), params.ProjectName)
	if err != nil {
		return "", fmt.Errorf("failed to create stack manager: %w", err)
	}
	pp := NewProviderPreparer(cliInterface, ec, client, sm)
	_, provider, err := pp.SetupProvider(ctx, sc.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	if params.ProjectName == "" {
		term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
		projectName, err := cliInterface.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			return "", fmt.Errorf("failed to load project name: %w", err)
		}
		params.ProjectName = projectName
	}

	if !pkg.IsValidSecretName(params.Name) {
		return "", fmt.Errorf("Invalid config name: secret name %q is not valid", params.Name)
	}

	value := params.Value
	if params.Random {
		value = cli.CreateRandomConfigValue()
		term.Debug("Generated random value for config")
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cliInterface.ConfigSet(ctx, params.ProjectName, provider, params.Name, value); err != nil {
		return "", fmt.Errorf("failed to set config: %w", err)
	}

	return fmt.Sprintf("Successfully set the config variable %q for project %q", params.Name, params.ProjectName), nil
}
