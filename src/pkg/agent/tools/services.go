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

func HandleServicesTool(ctx context.Context, loader cliClient.ProjectLoader, providerId *cliClient.ProviderID, cluster string, cli CLIInterface) (string, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return "", fmt.Errorf("no provider configured: %w", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	// Create a Defang client
	term.Debug("Function invoked: cli.NewProvider")
	provider, err := cli.NewProvider(ctx, *providerId, client)
	if err != nil {
		return "", fmt.Errorf("failed to create provider: %w", err)
	}

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
			return fmt.Sprintf("no services found for the specified project %q", projectName), nil
		}
		if connect.CodeOf(err) == connect.CodeNotFound && strings.Contains(err.Error(), "is not deployed in Playground") {
			return fmt.Sprintf("project %s is not deployed in Playground: %s", projectName, err.Error()), nil
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
