package tools

import (
	"context"
	"fmt"
	"time"

	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	"github.com/mark3labs/mcp-go/mcp"
)

type LogsParams struct {
	DeploymentID string
	Since        string
	Until        string
}

func parseLogsParams(request mcp.CallToolRequest) LogsParams {
	deploymentId := request.GetString("deployment_id", "")
	since := request.GetString("since", "")
	until := request.GetString("until", "")
	return LogsParams{
		DeploymentID: deploymentId,
		Since:        since,
		Until:        until,
	}
}

func handleLogsTool(ctx context.Context, loader cliClient.ProjectLoader, params LogsParams, cluster string, providerId *cliClient.ProviderID, cli CLIInterface) (string, error) {
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

	provider, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, "", len(project.Services))
	if err != nil {
		return "", fmt.Errorf("provider not configured correctly: %w", err)
	}

	sinceTime, err := timeutils.ParseTimeOrDuration(params.Since, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to parse 'since' parameter: %w", err)
	}

	untilTime, err := timeutils.ParseTimeOrDuration(params.Until, time.Now())
	if err != nil {
		return "", fmt.Errorf("failed to parse 'until' parameter: %w", err)
	}

	err = cli.Tail(ctx, provider, project, cliTypes.TailOptions{
		Deployment: params.DeploymentID,
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
