package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// handleRemoveConfigTool handles the remove config tool logic
func handleRemoveConfigTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli RemoveConfigCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Remove Config tool called")

	err := providerNotConfiguredError(*providerId)
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

	name, err := request.RequireString("name")
	if err != nil || name == "" {
		term.Error("Invalid config `name`", "error", errors.New("`name` is required"))
		return mcp.NewToolResultErrorFromErr("Invalid config `name`", errors.New("`name` is required")), err
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

	term.Debug("Function invoked: cli.ConfigDelete")
	if err := cli.ConfigDelete(ctx, projectName, provider, name); err != nil {
		// Show a warning (not an error) if the config was not found
		if connect.CodeOf(err) == connect.CodeNotFound {
			return mcp.NewToolResultText(fmt.Sprintf("Config variable %q not found in project %q", name, projectName)), nil
		}
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("Failed to remove config variable %q from project %q", name, projectName), err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully remove the config variable %q from project %q", name, projectName)), nil
}

// setupRemoveConfigTool configures and adds the estimate tool to the MCP server
func setupRemoveConfigTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
	term.Debug("Creating remove config tool")
	removeConfigTool := mcp.NewTool("remove_config",
		mcp.WithDescription("Remove a config variable for the defang project"),
		mcp.WithString("name",
			mcp.Description("The name of the config variable"),
			mcp.Required(),
		),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("remove config tool created")

	// Add the Config tool handler
	term.Debug("Adding remove config tool handler")
	cli := &DefaultToolCLI{}
	s.AddTool(removeConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		track.Evt("MCP Remove Config Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
		resp, err := handleRemoveConfigTool(ctx, request, providerId, cluster, &RemoveConfigCLIAdapter{DefaultToolCLI: cli})
		track.Evt("MCP Remove Config Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
		return resp, err
	})
}
