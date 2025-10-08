package tools

import (
	"context"
	"fmt"
	"os"
	"time"

	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

func handleLogsTool(ctx context.Context, request mcp.CallToolRequest, cluster string, providerId *cliClient.ProviderID, cli LogsCLIInterface) (string, error) {
	wd, err := request.RequireString("working_directory")
	if err != nil || wd == "" {
		return "", err
	}
	deploymentId, err := request.RequireString("deployment_id")
	if err != nil || deploymentId == "" {
		return "", err
	}
	since := request.GetString("since", "")
	until := request.GetString("until", "")
	sinceTime, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return "", fmt.Errorf("Invalid parameter 'since', must be in RFC3339 format: %w", err)
	}
	untilTime, err := time.Parse(time.RFC3339, until)
	if err != nil {
		return "", fmt.Errorf("Invalid parameter 'until', must be in RFC3339 format: %w", err)
	}

	err = os.Chdir(wd)
	if err != nil {
		return "", fmt.Errorf("failed to change working directory: %w", err)
	}

	loader := cli.ConfigureLoader(request)

	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)
		term.Error("Failed to deploy services", "error", err)

		return "", fmt.Errorf("local deployment failed: %v. Please provide a valid compose file path.", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")

	provider, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, len(project.Services))
	if err != nil {
		return "", fmt.Errorf("provider not configured correctly: %w", err)
	}

	err = cli.Tail(ctx, provider, project, cliTypes.TailOptions{
		Deployment: deploymentId,
		Since:      sinceTime,
		Until:      untilTime,
	})

	if err != nil {
		err = fmt.Errorf("failed to fetch logs: %w", err)
		term.Error("Failed to fetch logs", "error", err)
		return "", fmt.Errorf("failed to fetch logs: %w", err)
	}

	return "EOF", nil
}
