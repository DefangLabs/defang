package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	"github.com/mark3labs/mcp-go/mcp"
)

type LogsParams struct {
	common.LoaderParams
	DeploymentID string `json:"deployment_id" jsonschema:"description=Optional: Retrieve logs from a specific deployment."`
	Since        string `json:"since" jsonschema:"description=Optional: Retrieve logs written after this time. Format as RFC3339 or duration (e.g., '2023-10-01T15:04:05Z' or '1h')."`
	Until        string `json:"until" jsonschema:"description=Optional: Retrieve logs written before this time. Format as RFC3339 or duration (e.g., '2023-10-01T15:04:05Z' or '1h')."`
}

func ParseLogsParams(request mcp.CallToolRequest) (LogsParams, error) {
	deploymentId := request.GetString("deployment_id", "")
	since := request.GetString("since", "")
	until := request.GetString("until", "")
	return LogsParams{
		DeploymentID: deploymentId,
		Since:        since,
		Until:        until,
	}, nil
}

func HandleLogsTool(ctx context.Context, loader cliClient.ProjectLoader, params LogsParams, cluster string, providerId *cliClient.ProviderID, cli LogsCLIInterface) (string, error) {
	var sinceTime, untilTime time.Time
	var err error
	now := time.Now()
	if params.Since != "" {
		sinceTime, err = timeutils.ParseTimeOrDuration(params.Since, now)
		if err != nil {
			return "", fmt.Errorf("invalid parameter 'since', must be in RFC3339 format: %w", err)
		}
	}
	if params.Until != "" {
		untilTime, err = timeutils.ParseTimeOrDuration(params.Until, now)
		if err != nil {
			return "", fmt.Errorf("invalid parameter 'until', must be in RFC3339 format: %w", err)
		}
	}

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
