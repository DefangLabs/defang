package tools

import (
	"context"
	"fmt"
	"time"

	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
)

type LogsParams struct {
	common.LoaderParams
	DeploymentID string `json:"deployment_id,omitempty" jsonschema:"description=Optional: Retrieve logs from a specific deployment."`
	Since        string `json:"since,omitempty" jsonschema:"description=Optional: Retrieve logs written after this time. Format as RFC3339 or duration (e.g., '2023-10-01T15:04:05Z' or '1h')."`
	Until        string `json:"until,omitempty" jsonschema:"description=Optional: Retrieve logs written before this time. Format as RFC3339 or duration (e.g., '2023-10-01T15:04:05Z' or '1h')."`
}

func HandleLogsTool(ctx context.Context, loader cliClient.ProjectLoader, params LogsParams, cluster string, providerId *cliClient.ProviderID, cli CLIInterface) (string, error) {
	provider := cli.NewProvider(ctx, providerId, client, "")
	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("Failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

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

	err = cli.Tail(ctx, provider, projectName, cliTypes.TailOptions{
		Deployment:    params.DeploymentID,
		Since:         sinceTime,
		Until:         untilTime,
		Limit:         100,
		PrintBookends: true,
	})

	if err != nil {
		err = fmt.Errorf("failed to fetch logs: %w", err)
		term.Error("Failed to fetch logs", "error", err)
		return "", fmt.Errorf("failed to fetch logs: %w", err)
	}

	return "EOF", nil
}
