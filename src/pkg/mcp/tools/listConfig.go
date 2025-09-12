package tools

import (
	"context"
	"errors"
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

// ListConfigCLIInterface defines the methods needed for listing config variables
type ListConfigCLIInterface interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client *cliClient.GrpcClient) (cliClient.Provider, error)
	ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader
	LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error)
	ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error)
}

// DefaultListConfigCLI provides the default implementation
type DefaultListConfigCLI struct{}

func (c *DefaultListConfigCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultListConfigCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client *cliClient.GrpcClient) (cliClient.Provider, error) {
	return cli.NewProvider(ctx, providerId, client)
}

func (c *DefaultListConfigCLI) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}

func (c *DefaultListConfigCLI) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

func (c *DefaultListConfigCLI) ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error) {
	return provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: projectName})
}

// handleListConfigTool handles the list config tool logic
func handleListConfigTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli ListConfigCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("List Config tool called")
	track.Evt("MCP List Config Tool")

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
	for i, c := range config.Names {
		configNames[i] = c
	}

	return mcp.NewToolResultText(fmt.Sprintf("Here is the list of config variables for the project %q: %v", projectName, strings.Join(configNames, ", "))), nil
}

// setupSetConfigTool configures and adds the estimate tool to the MCP server
func setupListConfigTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
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
	cli := &DefaultListConfigCLI{}
	s.AddTool(listConfigTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListConfigTool(ctx, request, providerId, cluster, cli)
	})
}
