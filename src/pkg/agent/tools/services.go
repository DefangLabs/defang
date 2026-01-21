package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

type ServicesParams struct {
	common.LoaderParams
}

func HandleServicesTool(ctx context.Context, loader client.Loader, params ServicesParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
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
	term.Debugf("Project name loaded: %s", projectName)
	if err != nil {
		if strings.Contains(err.Error(), "no projects found") {
			return "no projects found on Playground", nil
		}
		return "", fmt.Errorf("failed to load project name: %w", err)
	}

	serviceResponse, err := cli.GetServices(ctx, projectName, provider)
	if err != nil {
		var noServicesErr cliTypes.ErrNoServices
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
