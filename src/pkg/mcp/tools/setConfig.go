package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupSetConfigTool configures and adds the estimate tool to the MCP server
func setupSetConfigTool(s *server.MCPServer, cluster string) {
	term.Debug("Creating set config tool")
	setConfigTool := mcp.NewTool("set_config",
		mcp.WithDescription("Set config variable for the defang project"),
		mcp.WithString("name",
			mcp.Description("The name of the config variable"),
			mcp.Required(),
		),

		mcp.WithString("value",
			mcp.Description("The value of the config variable"),
			mcp.Required(),
		),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("set config tool created")

	// Add the Config tool handler
	term.Debug("Adding set config tool handler")
	s.AddTool(setConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Debug("Set Config tool called")
		track.Evt("MCP Set Config Tool")

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

		value, ok := request.Params.Arguments["value"].(string)
		if !ok || value == "" {
			term.Debug("No value provided")
			return mcp.NewToolResultErrorFromErr("No value provided", errors.New("no value provided")), nil
		}

		term.Debug("Function invoked: cli.Connect")
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Could not connect", err), nil
		}

		term.Debug("Function invoked: cli.NewProvider")
		provider, err := cli.NewProvider(ctx, cliClient.ProviderDefang, client)
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

		if !pkg.IsValidSecretName(name) {
			return mcp.NewToolResultErrorFromErr("Invalid secret name", fmt.Errorf("secret name '%s' is not valid", name)), nil
		}

		term.Debug("Function invoked: cli.ConfigSet")
		if err := cli.ConfigSet(ctx, projectName, provider, name, value); err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to set config", err), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully set the config variable: %s to defang project: %s", name, projectName)), nil
	})
}
