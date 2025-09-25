package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/pkg/browser"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// OpenURLFunc allows browser.OpenURL to be overridden in tests
var OpenURLFunc = browser.OpenURL

// DefaultDeployCLI implements DeployCLIInterface using actual CLI functions
type DefaultDeployCLI struct{}

func (c *DefaultDeployCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultDeployCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client *cliClient.GrpcClient) (cliClient.Provider, error) {
	return cli.NewProvider(ctx, providerId, client)
}

func (c *DefaultDeployCLI) ComposeUp(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, uploadMode compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *compose.Project, error) {
	return cli.ComposeUp(ctx, project, client, provider, uploadMode, mode)
}

func (c *DefaultDeployCLI) Tail(ctx context.Context, projectName string, provider cliClient.Provider, options cli.TailOptions) error {
	return cli.Tail(ctx, provider, projectName, options)
}

func (c *DefaultDeployCLI) CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error) {
	return CheckProviderConfigured(ctx, client, providerId, projectName, serviceCount)
}

func (c *DefaultDeployCLI) LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (c *DefaultDeployCLI) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}

func (c *DefaultDeployCLI) OpenBrowser(url string) error {
	return browser.OpenURL(url)
}

func setupTailTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
	term.Debug("Creating tail tool")
	tailTool := mcp.NewTool("tail",
		mcp.WithDescription("Tail logs for a deployment."),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
		mcp.WithString("deployment_id",
			mcp.Description("The deployment ID to tail logs for"),
		),
	)
	term.Debug("Tail tool created")

	s.AddTool(tailTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cli := &DefaultToolCLI{}
		return handleTailTool(ctx, request, cluster, providerId, cli)
	})
}

func handleTailTool(ctx context.Context, request mcp.CallToolRequest, cluster string, providerId *cliClient.ProviderID, cli DeployCLIInterface) (*mcp.CallToolResult, error) {
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

	err = cli.Tail(ctx, provider, project.Name, cliTypes.TailOptions{
		Deployment: deploymentId,
		Since:      sinceTime,
		Until:      untilTime,
	})

	if err != nil {
		err = fmt.Errorf("failed to tail logs: %w", err)
		term.Error("Failed to tail logs", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to tail logs", err), err
	}

	return mcp.NewToolResultText("Done"), nil
}

// setupDeployTool configures and adds the deployment tool to the MCP server
func setupDeployTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
	term.Debug("Creating deployment tool")
	composeUpTool := mcp.NewTool("deploy",
		mcp.WithDescription("Deploy services using defang"),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("Deployment tool created")

	// Add the deployment tool handler - make it non-blocking
	term.Debug("Adding deployment tool handler")
	s.AddTool(composeUpTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cli := &DefaultToolCLI{}
		return handleDeployTool(ctx, request, providerId, cluster, cli)
	})
}

func handleDeployTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli DeployCLIInterface) (*mcp.CallToolResult, error) {
	err := providerNotConfiguredError(*providerId)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("No provider configured", err), err
	}

	// Get compose path
	term.Debug("Compose up tool called - deploying services")
	track.Evt("MCP Deploy Tool")

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

	// Deploy the services
	term.Debugf("Deploying services for project %s...", project.Name)

	term.Debug("Function invoked: cli.ComposeUp")
	// Use ComposeUp to deploy the services
	deployResp, project, err := cli.ComposeUp(ctx, project, client, provider, compose.UploadModeDigest, defangv1.DeploymentMode_DEVELOPMENT)
	if err != nil {
		err = fmt.Errorf("failed to compose up services: %w", err)
		term.Error("Failed to compose up services", "error", err)

		result := HandleTermsOfServiceError(err)
		if result != nil {
			return result, err
		}
		result = HandleConfigError(err)
		if result != nil {
			return result, err
		}

		return mcp.NewToolResultErrorFromErr("Failed to compose up services", err), err
	}

	if len(deployResp.Services) == 0 {
		term.Error("Failed to deploy services", "error", errors.New("no services deployed"))
		return mcp.NewToolResultText(fmt.Sprintf("Failed to deploy services: %v", errors.New("no services deployed"))), nil
	}

	// Get server from context for notifications
	mcpServer := server.ServerFromContext(ctx)
	go func() {
		count := 10
		for i := 0; i < 3; i++ {
			time.Sleep(1 * time.Second)
			if mcpServer == nil {
				return
			}

			mcpServer.SendNotificationToClient(ctx, "deployment_progress", map[string]any{
				"audience": "user",
				"progress": i,
				"total":    count,
				"message":  fmt.Sprintf("Processed %d/%d items", i, count),
			})
		}
	}()

	// Success case
	term.Debugf("Successfully started deployed services with etag: %s", deployResp.Etag)

	// Return the etag data as text
	return mcp.NewToolResultText(fmt.Sprintf("Deployment started. In order to follow the progress, tail the logs for deployment ID: %s", deployResp.Etag)), nil
}
