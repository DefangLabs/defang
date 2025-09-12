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

// SetConfigCLIInterface defines the CLI functions needed for setConfig tool
type SetConfigCLIInterface interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error)
	LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error)
	ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error
}

// DefaultSetConfigCLI implements SetConfigCLIInterface using the actual CLI functions
type DefaultSetConfigCLI struct{}

func (c *DefaultSetConfigCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultSetConfigCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error) {
	return cli.NewProvider(ctx, providerId, client)
}

func (c *DefaultSetConfigCLI) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

func (c *DefaultSetConfigCLI) ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error {
	return cli.ConfigSet(ctx, projectName, provider, name, value)
}

// setupSetConfigTool configures and adds the estimate tool to the MCP server
func setupSetConfigTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
	term.Debug("Creating set config tool")
	setConfigTool := mcp.NewTool("set_config",
		mcp.WithDescription("Set a config variable for the defang project"),
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
		cli := &DefaultSetConfigCLI{}
		return handleSetConfig(ctx, request, cluster, *providerId, cli)
	})
}

// handleSetConfig handles the set config MCP tool request
func handleSetConfig(ctx context.Context, request mcp.CallToolRequest, cluster string, providerId cliClient.ProviderID, cli SetConfigCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Set Config tool called")
	track.Evt("MCP Set Config Tool")

	err := providerNotConfiguredError(providerId)
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

	value, err := request.RequireString("value")
	if err != nil || value == "" {
		term.Error("Invalid config `value`", "error", errors.New("`value` is required"))
		return mcp.NewToolResultErrorFromErr("Invalid config `value`", errors.New("`value` is required")), err
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Could not connect", err), err
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider, err := cli.NewProvider(ctx, providerId, client)
	if err != nil {
		term.Error("Failed to get new provider", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to get new provider", err), err
	}

	loader := configureLoader(request)

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to load project name", err), err
	}
	term.Debug("Project name loaded:", projectName)

	if !pkg.IsValidSecretName(name) {
		return mcp.NewToolResultErrorFromErr("Invalid secret name", fmt.Errorf("secret name %q is not valid", name)), err
	}

	term.Debug("Function invoked: cli.ConfigSet")
	if err := cli.ConfigSet(ctx, projectName, provider, name, value); err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to set config", err), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully set the config variable %q for project %q", name, projectName)), nil
}
