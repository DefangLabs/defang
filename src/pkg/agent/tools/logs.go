package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
)

type LogsParams struct {
	common.LoaderParams
	DeploymentID string `json:"deployment_id,omitempty" jsonschema:"description=Optional: Retrieve logs from a specific deployment."`
	Since        string `json:"since,omitempty" jsonschema:"description=Optional: Retrieve logs written after this time. Format as RFC3339 or duration (e.g., '2023-10-01T15:04:05Z' or '1h')."`
	Until        string `json:"until,omitempty" jsonschema:"description=Optional: Retrieve logs written before this time. Format as RFC3339 or duration (e.g., '2023-10-01T15:04:05Z' or '1h')."`
}

func HandleLogsTool(ctx context.Context, loader cliClient.ProjectLoader, params LogsParams, cli CLIInterface, ec elicitations.Controller, config StackConfig) (string, error) {
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

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, config.Cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	sm := stacks.NewManager(params.WorkingDirectory)
	pp := NewProviderPreparer(cli, ec, client, sm)
	_, provider, err := pp.SetupProvider(ctx, config.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}
	term.Debug("Project name loaded:", projectName)

	err = cli.CanIUseProvider(ctx, client, projectName, config.Stack.Name, provider, 0)
	if err != nil {
		return "", fmt.Errorf("failed to use provider: %w", err)
	}

	err = cli.Tail(ctx, provider, projectName, cliTypes.TailOptions{
		Deployment:    params.DeploymentID,
		Since:         sinceTime,
		Until:         untilTime,
		Limit:         100,
		LogType:       logs.LogTypeAll,
		PrintBookends: true,
		Verbose:       true,
	})

	if err != nil {
		term.Error("Failed to fetch logs", "error", err)
		return "", fmt.Errorf("failed to fetch logs: %w", err)
	}

	return "", nil
}
