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
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

type ServicesParams struct {
	common.LoaderParams
}

func HandleServicesTool(ctx context.Context, loader cliClient.ProjectLoader, providerId *cliClient.ProviderID, cluster string, cli CLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", err
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	// Create a Defang client
	term.Debug("Function invoked: cli.NewProvider")
	provider := cli.NewProvider(ctx, *providerId, client, "")

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	term.Debugf("Project name loaded: %s", projectName)
	if err != nil {
		if strings.Contains(err.Error(), "no projects found") {
			return "", fmt.Errorf("no projects found on Playground: %w", err)
		}
		return "", fmt.Errorf("failed to load project name: %w", err)
	}

	serviceResponse, err := cli.GetServices(ctx, projectName, provider)
	if err != nil {
		var noServicesErr defangcli.ErrNoServices
		if errors.As(err, &noServicesErr) {
			return "", fmt.Errorf("no services found for the specified project %s: %w", projectName, err)
		}
		if connect.CodeOf(err) == connect.CodeNotFound && strings.Contains(err.Error(), "is not deployed in Playground") {
			return "", fmt.Errorf("project %s is not deployed in Playground: %w", projectName, err)
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
