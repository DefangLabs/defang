package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupRemoveConfigTool configures and adds the estimate tool to the MCP server
func setupRemoveConfigTool(s *server.MCPServer, cluster string, providerId cliClient.ProviderID) {
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
	s.AddTool(removeConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Debug("Remove Config tool called")
		track.Evt("MCP Remove Config Tool")

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if ok && wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
		}

		name, ok := request.Params.Arguments["name"].(string)
		if !ok || name == "" {
			term.Debug("No name provided")
			return mcp.NewToolResultErrorFromErr("No name provided", errors.New("no name provided")), nil
		}

		term.Debug("Function invoked: cli.Connect")
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Could not connect", err), nil
		}

		term.Debug("Function invoked: cli.NewProvider")
		provider, err := cli.NewProvider(ctx, providerId, client)
		if err != nil {
			term.Error("Failed to get new provider", "error", err)

			return mcp.NewToolResultErrorFromErr("Failed to get new provider", err), nil
		}

		loader := configureLoader(request)

		term.Debug("Function invoked: cliClient.LoadProjectNameWithFallback")
		projectName, err := cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to load project name", err), nil
		}
		term.Debug("Project name loaded:", projectName)

		term.Debug("Function invoked: cli.ConfigDelete")
		if err := cli.ConfigDelete(ctx, projectName, provider, name); err != nil {
			// Show a warning (not an error) if the config was not found
			if connect.CodeOf(err) == connect.CodeNotFound {
				return mcp.NewToolResultText(fmt.Sprintf("Config variable %q not found in project %q", name, projectName)), nil
			}
			return mcp.NewToolResultErrorFromErr(fmt.Sprintf("Failed to remove config variable %q from project %q", name, projectName), err), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully remove the config variable %q from project %q", name, projectName)), nil
	})
}
