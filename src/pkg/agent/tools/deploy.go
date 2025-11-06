package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func HandleDeployTool(
	ctx context.Context,
	loader cliClient.ProjectLoader,
	providerId *cliClient.ProviderID,
	cluster string, cli CLIInterface,
) (string, error) {
	var project *compose.Project
	var deployResp *defangv1.DeployResponse
	var provider *cliClient.Provider

	deployOutput, err := CaptureTerm(func() (string, error) {
		fmt.Printf("direct to stdout")
		term.Println("through term")
		err := common.ProviderNotConfiguredError(*providerId)
		if err != nil {
			return "", err
		}

		term.Debug("Function invoked: loader.LoadProject")
		proj, err := cli.LoadProject(ctx, loader)
		if err != nil {
			err = fmt.Errorf("failed to parse compose file: %w", err)

			return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
		}
		project = proj

		term.Debug("Function invoked: cli.Connect")
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			return "", fmt.Errorf("could not connect: %w", err)
		}

		term.Debug("Function invoked: cli.NewProvider")

		prov, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, "", len(project.Services))
		if err != nil {
			return "", fmt.Errorf("provider not configured correctly: %w", err)
		}
		provider = &prov

		// Deploy the services
		term.Debugf("Deploying services for project %s...", project.Name)

		term.Debug("Function invoked: cli.ComposeUp")
		// Use ComposeUp to deploy the services
		resp, _, err := cli.ComposeUp(ctx, project, client, prov, compose.UploadModeDigest, modes.ModeAffordable)
		if err != nil {
			err = fmt.Errorf("failed to compose up services: %w", err)

			err = common.FixupConfigError(err)
			return "", err
		}

		deployResp = resp

		if len(resp.Services) == 0 {
			return "", errors.New("no services deployed")
		}

		return "Deployment started", nil
	})
	if err != nil {
		return "", err
	}

	go func() {
		monitorOutput, err := CaptureTerm(func() (string, error) {
			_, err := cli.TailAndMonitor(ctx, project, *provider, 0, cliTypes.TailOptions{
				Follow:     true,
				Deployment: deployResp.Etag,
				Verbose:    false,
			})
			if err != nil {
				return "", err
			}
			return "Deployment completed", nil
		})
		if err != nil {
			term.Errorf("Error while monitoring deployment: %v", err)
		}

		term.Debugf("Deployment output:\n%s", monitorOutput)
		term.Println("Deployment completed.")
	}()

	// Success case
	urls := strings.Builder{}
	for _, serviceInfo := range deployResp.Services {
		if serviceInfo.PublicFqdn != "" {
			urls.WriteString(fmt.Sprintf("- %s: %s %s\n", serviceInfo.Service.Name, serviceInfo.PublicFqdn, serviceInfo.Domainname))
		}
	}

	// Return the etag data as text
	return fmt.Sprintf(`
%s

The deployment is now in progress. Check the logs to follow progress with deployment id %q.

When the deployment is complete, you will be able to access the services at the following URLs:
%s
`, deployOutput, deployResp.Etag, urls.String()), nil
}
