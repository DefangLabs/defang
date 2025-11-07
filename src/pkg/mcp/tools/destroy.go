package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

func handleDestroyTool(ctx context.Context, loader cliClient.ProjectLoader, providerId *cliClient.ProviderID, cluster string, cli CLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", err
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider := cli.NewProvider(ctx, *providerId, client, "")

	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}

	err = cli.CanIUseProvider(ctx, client, *providerId, projectName, provider, 0)
	if err != nil {
		return "", fmt.Errorf("failed to use provider: %w", err)
	}

	term.Debug("Function invoked: cli.ComposeDown")
	deployment, err := cli.ComposeDown(ctx, projectName, client, provider)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			// Show a warning (not an error) if the service was not found
			return "", errors.New("project not found, nothing to destroy. Please use a valid project name, compose file path or project directory")
		}

		return "", fmt.Errorf("failed to send destroy request: %w", err)
	}

	return fmt.Sprintf("The project is in the process of being destroyed: %s, please tail this deployment ID: %s for status updates.", projectName, deployment), nil
}
