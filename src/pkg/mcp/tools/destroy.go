package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
)

func handleDestroyTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli DestroyCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Compose down tool called - removing services")
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("No provider configured", err), err
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Could not connect", err), err
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider, err := cli.NewProvider(ctx, *providerId, client)
	if err != nil {
		term.Error("Failed to get new provider", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to get new provider", err), err
	}

	wd, err := request.RequireString("working_directory")
	if err != nil || wd == "" {
		term.Error("Invalid working directory", "error", errors.New("working_directory is required"))
		return mcp.NewToolResultErrorFromErr("Invalid working directory", errors.New("working_directory is required")), err
	}

	err = os.Chdir(wd)
	if err != nil {
		term.Error("Failed to change working directory", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to change working directory", err), err
	}

	loader := cli.ConfigureLoader(request)

	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		term.Error("Failed to load project name", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to load project name", err), err
	}

	err = cli.CanIUseProvider(ctx, client, *providerId, projectName, provider, 0)
	if err != nil {
		term.Error("Failed to use provider", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to use provider", err), err
	}

	term.Debug("Function invoked: cli.ComposeDown")
	deployment, err := cli.ComposeDown(ctx, projectName, client, provider)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			// Show a warning (not an error) if the service was not found
			term.Warn("Project not found", "error", err)
			return mcp.NewToolResultText("Project not found, nothing to destroy. Please use a valid project name, compose file path or project directory."), err
		}

		return mcp.NewToolResultErrorFromErr("Failed to send destroy request", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("The project is in the process of being destroyed: %s, please tail this deployment ID: %s for status updates.", projectName, deployment)), nil
}
