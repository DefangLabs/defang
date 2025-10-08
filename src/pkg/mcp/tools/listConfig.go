package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleListConfigTool handles the list config tool logic
func handleListConfigTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli ListConfigCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("List Config tool called")

	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("No provider configured", err), err
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

	loader := cli.ConfigureLoader(request)

	term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to load project name", err), err
	}
	term.Debug("Project name loaded:", projectName)

	term.Debug("Function invoked: cli.ConfigList")
	config, err := cli.ListConfig(ctx, provider, projectName)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to list config variables", err), err
	}

	numConfigs := len(config.Names)
	if numConfigs == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No config variables found for the project %q.", projectName)), nil
	}

	configNames := make([]string, numConfigs)
	copy(configNames, config.Names)

	return mcp.NewToolResultText(fmt.Sprintf("Here is the list of config variables for the project %q: %v", projectName, strings.Join(configNames, ", "))), nil
}
