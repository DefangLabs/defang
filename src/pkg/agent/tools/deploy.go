package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type DeployParams struct {
	common.LoaderParams
}

func HandleDeployTool(ctx context.Context, loader client.Loader, params DeployParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)

		return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
	}

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

	err = cli.CanIUseProvider(ctx, client, provider, project.Name, len(project.Services))
	if err != nil {
		return "", fmt.Errorf("failed to use provider: %w", err)
	}

	// Deploy the services
	term.Debugf("Deploying services for project %s...", project.Name)

	term.Debug("Function invoked: cli.ComposeUp")
	// Use ComposeUp to deploy the services
	deployResp, project, err := cli.ComposeUp(ctx, client, provider, sc.Stack, cliTypes.ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModeDigest,
		Mode:       modes.ModeAffordable,
	})
	if err != nil {
		err = fmt.Errorf("failed to compose up services: %w", err)

		var missing compose.ErrMissingConfig
		if errors.As(err, &missing) && ec.IsSupported() {
			err := requestMissingConfig(ctx, ec, cli, provider, project.Name, missing)
			if err != nil {
				return "", fmt.Errorf("failed to request missing config: %w", err)
			}

			// try again
			return HandleDeployTool(ctx, loader, params, cli, ec, sc)
		}

		return "", err
	}

	if len(deployResp.Services) == 0 {
		return "", errors.New("no services deployed")
	}

	urls := strings.Builder{}
	for _, serviceInfo := range deployResp.Services {
		if serviceInfo.PublicFqdn != "" {
			urls.WriteString(fmt.Sprintf("- %s: %s %s\n", serviceInfo.Service.Name, serviceInfo.PublicFqdn, serviceInfo.Domainname))
		}
	}

	return fmt.Sprintf(
		"The deployment is not complete, but it has been started successfully.\n"+
			"To follow progress, tail the logs for deployment %q.\n"+
			"Your application will be available at the following url(s) when the deployment is complete:\n"+
			"%s\n",
		deployResp.Etag,
		urls.String(),
	), nil
}

func requestMissingConfig(ctx context.Context, ec elicitations.Controller, cli CLIInterface, provider client.Provider, projectName string, names []string) error {
	for _, name := range names {
		value, err := ec.RequestString(ctx, "This config value needs to be set", name)
		if err != nil {
			return fmt.Errorf("failed to request config %q: %w", name, err)
		}

		err = cli.ConfigSet(ctx, projectName, provider, name, value)
		if err != nil {
			return fmt.Errorf("failed to set config %q: %w", name, err)
		}
	}

	return nil
}
