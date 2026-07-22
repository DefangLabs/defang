package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type SetConfigParams struct {
	common.LoaderParams
	Name   string `json:"name" jsonschema:"required"`
	Value  string `json:"value,omitempty"`
	Random bool   `json:"random,omitempty" jsonschema:"description=Generate a secure randomly generated value for config (default: false)"`
}

func HandleSetConfig(ctx context.Context, params SetConfigParams, cliInterface CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	_, provider, loader, err := setupProviderAndLoader(ctx, params.LoaderParams, cliInterface, ec, sc)
	if err != nil {
		return setupErrorResult(err)
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
		if params.Value != "" {
			return "", errors.New("Both 'random' and 'value' parameters provided; please provide only one")
		}
		value = cli.CreateRandomConfigValue()
		term.Debug("Generated random value for config")
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cliInterface.ConfigSet(ctx, params.ProjectName, provider, params.Name, value); err != nil {
		return "", fmt.Errorf("failed to set config: %w", err)
	}

	return fmt.Sprintf("Successfully set the config variable %q for project %q in stack %q", params.Name, params.ProjectName, sc.Stack.Name), nil
}
