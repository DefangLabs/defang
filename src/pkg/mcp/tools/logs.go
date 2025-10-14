package tools

import (
	"context"
	"fmt"
	"time"

	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

type LogsParams struct {
	DeploymentID string
	Since        time.Time
	Until        time.Time
}

func parseLogsParams(request mcp.CallToolRequest) (LogsParams, error) {
	deploymentId := request.GetString("deployment_id", "")
	since, err := request.RequireString("since")
	if err != nil {
		return LogsParams{}, fmt.Errorf("missing required parameter 'since': %w", err)
	}
	until, err := request.RequireString("until")
	if err != nil {
		return LogsParams{}, fmt.Errorf("missing required parameter 'until': %w", err)
	}
	var sinceTime, untilTime time.Time
	if since != "" {
		sinceTime, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return LogsParams{}, fmt.Errorf("invalid parameter 'since', must be in RFC3339 format: %w", err)
		}
	}
	if until != "" {
		untilTime, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return LogsParams{}, fmt.Errorf("invalid parameter 'until', must be in RFC3339 format: %w", err)
		}
	}
	return LogsParams{
		DeploymentID: deploymentId,
		Since:        sinceTime,
		Until:        untilTime,
	}, nil
}

func handleLogsTool(ctx context.Context, loader cliClient.ProjectLoader, params LogsParams, cluster string, providerId *cliClient.ProviderID, cli LogsCLIInterface) (string, error) {
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
		Deployment: params.DeploymentID,
		Since:      params.Since,
		Until:      params.Until,
	})

	if err != nil {
		err = fmt.Errorf("failed to fetch logs: %w", err)
		term.Error("Failed to fetch logs", "error", err)
		return "", fmt.Errorf("failed to fetch logs: %w", err)
	}

	return "EOF", nil
}
