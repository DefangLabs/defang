package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func HandleDeployTool(ctx context.Context, loader cliClient.ProjectLoader, providerId *cliClient.ProviderID, cluster string, cli CLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", err
	}

	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)

		return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")

	provider, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, "", len(project.Services))
	if err != nil {
		return "", fmt.Errorf("provider not configured correctly: %w", err)
	}

	// Deploy the services
	term.Debugf("Deploying services for project %s...", project.Name)

	term.Debug("Function invoked: cli.ComposeUp")
	// Use ComposeUp to deploy the services
	deployResp, project, err := cli.ComposeUp(ctx, client, provider, cliTypes.ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModeDigest,
		Mode:       modes.ModeAffordable,
	})
	if err != nil {
		err = fmt.Errorf("failed to compose up services: %w", err)

		err = common.FixupConfigError(err)
		return "", err
	}

	if len(deployResp.Services) == 0 {
		return "", errors.New("no services deployed")
	}

	term.Println("Starting deployment_id:", deployResp.Etag)

	_, err = cli.TailAndMonitor(ctx, project, provider, 0, cliTypes.TailOptions{
		Follow:     true,
		Deployment: deployResp.Etag,
		Verbose:    true,
		LogType:    logs.LogTypeAll,
		Raw:        true,
	})
	if err != nil {
		return "", err
	}

	return "Deployment completed successfully", nil
}
