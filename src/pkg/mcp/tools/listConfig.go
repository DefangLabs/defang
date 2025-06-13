package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupSetConfigTool configures and adds the estimate tool to the MCP server
func setupListConfigTool(s *server.MCPServer, cluster string) {
	term.Debug("Creating list config tool")
	listConfigTool := mcp.NewTool("list_configs",
		mcp.WithDescription("List all config variables for the defang project"),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("list config tool created")

	// Add the Config tool handler
	term.Debug("Adding List config tool handler")
	s.AddTool(listConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Debug("List Config tool called")
		track.Evt("MCP List Config Tool")

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if ok && wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
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

		term.Debug("Function invoked: cli.ConfigList")

		config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: projectName})
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to list config variables", err), nil
		}

		numConfigs := len(config.Names)
		if numConfigs == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No config variables found for the project %q.", projectName)), nil
		}

		configNames := make([]string, numConfigs)
		for i, c := range config.Names {
			configNames[i] = c
		}

		return mcp.NewToolResultText(fmt.Sprintf("Here is the list of config variables for the project %q: %v", projectName, strings.Join(configNames, ", "))), nil
	})
}
