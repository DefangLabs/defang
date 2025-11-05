package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
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
	deployResp, _, err := cli.ComposeUp(ctx, project, client, provider, compose.UploadModeDigest, modes.ModeAffordable)
	if err != nil {
		err = fmt.Errorf("failed to compose up services: %w", err)

		err = common.FixupConfigError(err)
		return "", err
	}

	if len(deployResp.Services) == 0 {
		return "", errors.New("no services deployed")
	}

	// Success case
	term.Debugf("Successfully started deployed services with etag: %s", deployResp.Etag)
	urls := strings.Builder{}
	for _, serviceInfo := range deployResp.Services {
		if serviceInfo.PublicFqdn != "" {
			urls.WriteString(fmt.Sprintf("- %s: %s %s\n", serviceInfo.Service.Name, serviceInfo.PublicFqdn, serviceInfo.Domainname))
		}
	}

	// Return the etag data as text
	return fmt.Sprintf("The deployment is now in progress. Check the logs to follow progress with deployment id %q. When the deployment is complete, you will be able to access the services at the following URLs:\n%s", deployResp.Etag, urls.String()), nil
}
