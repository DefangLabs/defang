package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/browser"

	"github.com/DefangLabs/defang/src/pkg/cli"
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

// setupDeployTool configures and adds the deployment tool to the MCP server
func setupDeployTool(s *server.MCPServer, cluster string, providerId *cliClient.ProviderID) {
	term.Debug("Creating deployment tool")
	composeUpTool := mcp.NewTool("deploy",
		mcp.WithDescription("Deploy services using defang"),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory; required"),
			mcp.Required(),
		),

		mcp.WithArray("compose_file_paths",
			mcp.Description("Paths to docker-compose files; optional"),
			mcp.Items(map[string]any{"type": "string"}),
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

	// Success case
	term.Debugf("Successfully started deployed services with etag: %s", deployResp.Etag)

	// Log deployment success
	term.Debug("Deployment Started!")
	term.Debugf("Deployment ID: %s", deployResp.Etag)

	var portal string
	if *providerId == cliClient.ProviderDefang {
		// Get the portal URL for browser preview
		portalURL := "https://portal.defang.io/"

		// Open the portal URL in the browser
		term.Debugf("Opening portal URL in browser: %s", portalURL)
		go func() {
			err := OpenURLFunc(portalURL)
			if err != nil {
				term.Error("Failed to open URL in browser", "error", err, "url", portalURL)
			}
		}()

		// Log browser preview information
		term.Debugf("🌐 %s available", portalURL)
		portal = "Please use the web portal url: %s" + portalURL
	} else {
		// portalURL := fmt.Sprintf("https://%s.signin.aws.amazon.com/console")
		portal = fmt.Sprintf("Please use the %s console", providerId)
	}

	// Log service details
	term.Debug("Services:")
	for _, serviceInfo := range deployResp.Services {
		term.Debugf("- %s", serviceInfo.Service.Name)
		term.Debugf("  Public URL: %s", serviceInfo.PublicFqdn)
		term.Debugf("  Status: %s", serviceInfo.Status)
	}

	urls := strings.Builder{}
	for _, serviceInfo := range deployResp.Services {
		if serviceInfo.PublicFqdn != "" {
			urls.WriteString(fmt.Sprintf("- %s: %s %s\n", serviceInfo.Service.Name, serviceInfo.PublicFqdn, serviceInfo.Domainname))
		}
	}

	// Return the etag data as text
	return mcp.NewToolResultText(fmt.Sprintf("%s to follow the deployment of %s, with the deployment ID of %s:\n%s", portal, project.Name, deployResp.Etag, urls.String())), nil
}
