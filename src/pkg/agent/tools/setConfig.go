package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type SetConfigParams struct {
	common.LoaderParams
	Name  string `json:"name" jsonschema:"required"`
	Value string `json:"value" jsonschema:"required"`
}

func HandleSetConfig(ctx context.Context, loader cliClient.ProjectLoader, params SetConfigParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, sc.Cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	pp := NewProviderPreparer(cli, ec, client)
	if params.ProjectName == "" {
		projectName, err := cli.LoadProjectName(ctx, loader)
		if err != nil {
			return "", fmt.Errorf("failed to load project name: %w", err)
		}
		params.ProjectName = projectName
	}

	_, provider, err := pp.SetupProvider(ctx, params.ProjectName, sc.Stack, false)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	if !pkg.IsValidSecretName(params.Name) {
		return "", fmt.Errorf("Invalid config name: secret name %q is not valid", params.Name)
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cli.ConfigSet(ctx, params.ProjectName, provider, params.Name, params.Value); err != nil {
		return "", fmt.Errorf("failed to set config: %w", err)
	}

	return fmt.Sprintf("Successfully set the config variable %q for project %q", params.Name, params.ProjectName), nil
}
