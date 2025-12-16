package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	defangcli "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

type ServicesParams struct {
	common.LoaderParams
}

func HandleServicesTool(ctx context.Context, loader cliClient.Loader, params ServicesParams, cli CLIInterface, ec elicitations.Controller, config StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, config.Cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	sm, err := stacks.NewManager(client, params.WorkingDirectory, params.ProjectName)
	if err != nil {
		return "", fmt.Errorf("failed to create stack manager: %w", err)
	}
	pp := NewProviderPreparer(cli, ec, client, sm)
	_, provider, err := pp.SetupProvider(ctx, config.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}
	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	term.Debugf("Project name loaded: %s", projectName)
	if err != nil {
		if strings.Contains(err.Error(), "no projects found") {
			return "no projects found on Playground", nil
		}
		return "", fmt.Errorf("failed to load project name: %w", err)
	}

	serviceResponse, err := cli.GetServices(ctx, projectName, provider)
	if err != nil {
		var noServicesErr defangcli.ErrNoServices
		if errors.As(err, &noServicesErr) {
			return noServicesErr.Error(), nil
		}
		if connect.CodeOf(err) == connect.CodeNotFound && strings.Contains(err.Error(), "is not deployed in Playground") {
			return fmt.Sprintf("project %s is not deployed in Playground", projectName), nil
		}

		return "", fmt.Errorf("failed to get services: %w", err)
	}

	// Convert to JSON
	jsonData, jsonErr := json.Marshal(serviceResponse)
	if jsonErr == nil {
		term.Debugf("Successfully loaded services with count: %d", len(serviceResponse))
		return string(jsonData) + "\nIf you would like to see more details about your deployed projects, please visit the Defang portal at https://portal.defang.io/projects", nil
	}

	// Return the data in a structured format
	return "Successfully loaded services, but failed to convert to JSON. Please check the logs for details.", nil
}
