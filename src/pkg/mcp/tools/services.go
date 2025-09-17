package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	defangcli "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DefaultDeploymentInfo implements DeploymentInfoInterface using the actual deployment_info functions
type DefaultDeploymentInfo struct{}

func (d *DefaultDeploymentInfo) GetServices(ctx context.Context, projectName string, provider cliClient.Provider) ([]deployment_info.Service, error) {
	return deployment_info.GetServices(ctx, projectName, provider)
}

// DefaultCLI implements CLIInterface using the actual CLI functions
type DefaultCLI struct{}

func (c *DefaultCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return defangcli.Connect(ctx, cluster)
}

func (c *DefaultCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error) {
	return defangcli.NewProvider(ctx, providerId, client)
}

func (c *DefaultCLI) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

// setupServicesTool configures and adds the services tool to the MCP server
func setupServicesTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
	term.Debug("Creating services tool")
	servicesTool := mcp.NewTool("services",
		mcp.WithDescription("List information about services in Defang Playground"),
		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("Services tool created")

	// Add the services tool handler - make it non-blocking
	term.Debug("Adding services tool handler")
	s.AddTool(servicesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cli := &DefaultCLI{}
		deploymentInfo := &DefaultDeploymentInfo{}
		return handleServicesTool(ctx, request, providerId, cluster, cli, deploymentInfo)
	})
}

func handleServicesTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli CLIInterface, deploymentInfo DeploymentInfoInterface) (*mcp.CallToolResult, error) {
	term.Debug("Services tool called - fetching services from Defang")
	track.Evt("MCP Services Tool")

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

	loader := configureLoader(request)

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Could not connect", err), err
	}

	// Create a Defang client
	term.Debug("Function invoked: cli.NewProvider")
	provider, err := cli.NewProvider(ctx, *providerId, client)
	if err != nil {
		term.Error("Failed to create provider", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to create provider", err), err
	}

	term.Debug("Function invoked: cli.LoadProjectNameWithFallback")
	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	term.Debugf("Project name loaded: %s", projectName)
	if err != nil {
		if strings.Contains(err.Error(), "no projects found") {
			term.Errorf("No projects found on Playground, error: %v", err)
			return mcp.NewToolResultText("No projects found on Playground"), nil
		}
		term.Errorf("Failed to load project name, error: %v", err)
		return mcp.NewToolResultErrorFromErr("Failed to load project name", err), err
	}

	serviceResponse, err := deploymentInfo.GetServices(ctx, projectName, provider)
	if err != nil {
		var noServicesErr defangcli.ErrNoServices
		if errors.As(err, &noServicesErr) {
			term.Warnf("No services found for the specified project %s", projectName)
			return mcp.NewToolResultText("No services found for the specified project " + projectName), err
		}
		if connect.CodeOf(err) == connect.CodeNotFound && strings.Contains(err.Error(), "is not deployed in Playground") {
			term.Warnf("Project %s is not deployed in Playground", projectName)
			return mcp.NewToolResultText(fmt.Sprintf("Project %s is not deployed in Playground", projectName)), err
		}

		result := HandleTermsOfServiceError(err)
		if result != nil {
			return result, err
		}

		term.Error("Failed to get services", "error", err)
		return mcp.NewToolResultText("Failed to get services"), nil
	}

	// Convert to JSON
	jsonData, jsonErr := json.Marshal(serviceResponse)
	if jsonErr == nil {
		term.Debugf("Successfully loaded services with count: %d", len(serviceResponse))
		// Use NewToolResultText with JSON string
		return mcp.NewToolResultText(string(jsonData) + "\nIf you would like to see more details about your deployed projects, please visit the Defang portal at https://portal.defang.io/projects"), nil
	}

	// Return the data in a structured format
	return mcp.NewToolResultText("Successfully loaded services, but failed to convert to JSON. Please check the logs for details."), nil
}
