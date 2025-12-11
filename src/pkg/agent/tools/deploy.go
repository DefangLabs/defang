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
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type DeployParams struct {
	common.LoaderParams
}

func HandleDeployTool(ctx context.Context, loader cliClient.ProjectLoader, cli CLIInterface, ec elicitations.Controller, config StackConfig) (string, error) {
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

	pp := NewProviderPreparer(cli, ec, client)
	providerID, provider, err := pp.SetupProvider(ctx, project.Name, config.Stack, true)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	err = cli.CanIUseProvider(ctx, client, *providerID, project.Name, provider, len(project.Services))
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
	})
	if err != nil {
		err = fmt.Errorf("failed to compose up services: %w", err)

		var missing compose.ErrMissingConfig
		if errors.As(err, &missing) {
			err := requestMissingConfig(ctx, ec, cli, provider, project.Name, missing)
			if err != nil {
				return "", fmt.Errorf("failed to request missing config: %w", err)
			}

			// try again
			return HandleDeployTool(ctx, loader, cli, ec, config)
		}

		return "", err
	}

	if len(deployResp.Services) == 0 {
		return "", errors.New("no services deployed")
	}

	term.Debugf("Deployment ID: %s", deployResp.Etag)

	_, err = cli.TailAndMonitor(ctx, project, provider, 0, cliTypes.TailOptions{
		Follow:     true,
		Deployment: deployResp.Etag,
		Verbose:    true,
		LogType:    logs.LogTypeAll,
		Raw:        true,
	})
	if err != nil {
		return "", fmt.Errorf("error during deployment %q: %w", deployResp.Etag, err)
	}

	urls := strings.Builder{}
	for _, serviceInfo := range deployResp.Services {
		if serviceInfo.PublicFqdn != "" {
			urls.WriteString(fmt.Sprintf("- %s: %s %s\n", serviceInfo.Service.Name, serviceInfo.PublicFqdn, serviceInfo.Domainname))
		}
	}

	return fmt.Sprintf("Deployment %q completed successfully\n%s", deployResp.Etag, urls.String()), nil
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
