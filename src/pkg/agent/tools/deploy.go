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
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type DeployParams struct {
	common.LoaderParams
}

func HandleDeployTool(ctx context.Context, loader cliClient.ProjectLoader, params DeployParams, cli CLIInterface, ec elicitations.Controller, config StackConfig) (string, error) {
	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)

		return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, config.Cluster)
	if err != nil {
		err = cli.InteractiveLoginMCP(ctx, client, config.Cluster, common.MCPDevelopmentClient)
		if err != nil {
			var noBrowserErr auth.ErrNoBrowser
			if errors.As(err, &noBrowserErr) {
				return noBrowserErr.Error(), nil
			}
			return "", err
		}
	}

	sm := stacks.NewManager(client, params.WorkingDirectory, "")
	pp := NewProviderPreparer(cli, ec, client, sm)
	_, provider, err := pp.SetupProvider(ctx, config.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	err = cli.CanIUseProvider(ctx, client, project.Name, config.Stack.Name, provider, len(project.Services))
	if err != nil {
		return "", fmt.Errorf("failed to use provider: %w", err)
	}

	// Deploy the services
	term.Debugf("Deploying services for project %s...", project.Name)

	term.Debug("Function invoked: cli.ComposeUp")
	// Use ComposeUp to deploy the services
	deployResp, project, err := cli.ComposeUp(ctx, client, provider, cliTypes.ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModeDigest,
		Mode:       modes.ModeAffordable,
		Stack:      config.Stack.Name,
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
			return HandleDeployTool(ctx, loader, params, cli, ec, config)
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
