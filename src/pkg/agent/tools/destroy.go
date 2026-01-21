package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

type DestroyParams struct {
	common.LoaderParams
}

func HandleDestroyTool(ctx context.Context, loader client.Loader, params DestroyParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := GetClientWithRetry(ctx, cli, sc)
	if err != nil {
		var noBrowserErr auth.ErrNoBrowser
		if errors.As(err, &noBrowserErr) {
			return noBrowserErr.Error(), nil
		}
		return "", err
	}

	sm, err := stacks.NewManager(client, loader.TargetDirectory(), params.ProjectName, ec)
	if err != nil {
		return "", fmt.Errorf("failed to create stack manager: %w", err)
	}
	pp := NewProviderPreparer(cli, ec, client, sm)
	_, provider, err := pp.SetupProvider(ctx, sc.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}
	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}

	err = cli.CanIUseProvider(ctx, client, provider, projectName, 0)
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
