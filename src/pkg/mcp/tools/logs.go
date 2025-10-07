package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
)

// DefaultLogsCLI implements LogsCLIInterface using actual CLI functions
type DefaultLogsCLI struct{}

func (c *DefaultLogsCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultLogsCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client *cliClient.GrpcClient) (cliClient.Provider, error) {
	return cli.NewProvider(ctx, providerId, client)
}

func (c *DefaultLogsCLI) Tail(ctx context.Context, provider cliClient.Provider, project *compose.Project, options cliTypes.TailOptions) error {
	return cli.Tail(ctx, provider, project.Name, options)
}

func (c *DefaultLogsCLI) CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error) {
	return CheckProviderConfigured(ctx, client, providerId, projectName, serviceCount)
}

func (c *DefaultLogsCLI) LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (c *DefaultLogsCLI) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}

func handleLogsTool(ctx context.Context, request mcp.CallToolRequest, cluster string, providerId *cliClient.ProviderID, cli LogsCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Tail tool called - opening logs in browser")
	track.Evt("MCP Tail Tool")
	wd, err := request.RequireString("working_directory")
	if err != nil || wd == "" {
		term.Error("Invalid working directory", "error", errors.New("working_directory is required"))
		return mcp.NewToolResultErrorFromErr("Invalid working directory", errors.New("working_directory is required")), err
	}
	deploymentId, err := request.RequireString("deployment_id")
	if err != nil || deploymentId == "" {
		term.Error("Invalid deployment ID", "error", errors.New("deployment_id is required"))
		return mcp.NewToolResultErrorFromErr("Invalid deployment ID", errors.New("deployment_id is required")), err
	}
	since := request.GetString("since", "")
	until := request.GetString("until", "")
	sinceTime, err := time.Parse(time.RFC3339, since)
	if err != nil {
		term.Error("Invalid parameter 'since', must be in RFC3339 format", "error", err)
		return mcp.NewToolResultErrorFromErr("Invalid parameter 'since', must be in RFC3339 format", err), err
	}
	untilTime, err := time.Parse(time.RFC3339, until)
	if err != nil {
		term.Error("Invalid parameter 'until', must be in RFC3339 format", "error", err)
		return mcp.NewToolResultErrorFromErr("Invalid parameter 'until', must be in RFC3339 format", err), err
	}

	err = os.Chdir(wd)
	if err != nil {
		term.Error("Failed to change working directory", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to change working directory", err), err
	}

	loader := cli.ConfigureLoader(request)

	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)
		term.Error("Failed to deploy services", "error", err)

		return mcp.NewToolResultText(fmt.Sprintf("Local deployment failed: %v. Please provide a valid compose file path.", err)), err
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Could not connect", err), err
	}

	term.Debug("Function invoked: cli.NewProvider")

	provider, err := cli.CheckProviderConfigured(ctx, client, *providerId, project.Name, len(project.Services))
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Provider not configured correctly", err), err
	}

	err = cli.Tail(ctx, provider, project, cliTypes.TailOptions{
		Deployment: deploymentId,
		Since:      sinceTime,
		Until:      untilTime,
	})

	if err != nil {
		err = fmt.Errorf("failed to fetch logs: %w", err)
		term.Error("Failed to fetch logs", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to fetch logs", err), err
	}

	return mcp.NewToolResultText("Done"), nil
}
