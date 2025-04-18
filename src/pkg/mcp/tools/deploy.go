package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pkg/browser"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupDeployTool configures and adds the deployment tool to the MCP server
func setupDeployTool(s *server.MCPServer, client client.GrpcClient) {
	term.Info("Creating deployment tool")
	composeUpTool := mcp.NewTool("deploy",
		mcp.WithDescription("Deploy services using defang"),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("Deployment tool created")

	// Add the deployment tool handler - make it non-blocking
	term.Info("Adding deployment tool handler")
	s.AddTool(composeUpTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get compose path
		term.Info("Compose up tool called - deploying services")

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if ok && wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
		}

		loader := configureLoader(request)

		project, err := loader.LoadProject(ctx)
		if err != nil {
			err = fmt.Errorf("failed to parse compose file: %w", err)
			term.Error("Failed to deploy services", "error", err)

			return mcp.NewToolResultText(fmt.Sprintf("Local deployment failed: %v. Please provide a valid compose file path.", err)), nil
		}

		provider, err := cli.NewProvider(ctx, cliClient.ProviderDefang, client)
		if err != nil {
			term.Error("Failed to get new provider", "error", err)

			return mcp.NewToolResultText(fmt.Sprintf("Failed to get new provider: %v", err)), nil
		}

		// Deploy the services
		term.Infof("Deploying services for project %s...", project.Name)

		// Use ComposeUp to deploy the services
		deployResp, project, err := cli.ComposeUp(ctx, project, client, provider, compose.UploadModeDigest, defangv1.DeploymentMode_DEVELOPMENT)
		if err != nil {
			err = fmt.Errorf("failed to compose up services: %w", err)
			term.Error("Failed to compose up services", "error", err)

			return mcp.NewToolResultText(fmt.Sprintf("Failed to compose up services: %v", err)), nil
		}

		if len(deployResp.Services) == 0 {
			term.Error("Failed to deploy services", "error", errors.New("no services deployed"))
			return mcp.NewToolResultText(fmt.Sprintf("Failed to deploy services: %v", errors.New("no services deployed"))), nil
		}

		// Get the portal URL for browser preview
		portalURL := printPlaygroundPortalServiceURLs(deployResp.Services)

		// Open the portal URL in the browser if available
		if portalURL != "" {
			term.Infof("Opening portal URL in browser: %s", portalURL)
			go func() {
				err := browser.OpenURL(portalURL)
				if err != nil {
					term.Error("Failed to open URL in browser", "error", err, "url", portalURL)
				}
			}()
		}

		// Success case
		term.Info("Successfully started deployed services", "etag", deployResp.Etag)

		// Log deployment success
		term.Info("Deployment Started!")
		term.Infof("Deployment ID: %s", deployResp.Etag)

		// Log browser preview information
		term.Infof("🌐 %s available", portalURL)

		// Log service details
		term.Info("Services:")
		for _, serviceInfo := range deployResp.Services {
			term.Infof("- %s", serviceInfo.Service.Name)
			term.Infof("  Public URL: %s", serviceInfo.PublicFqdn)
			term.Infof("  Status: %s", serviceInfo.Status)
		}

		// Return the etag data as text
		return mcp.NewToolResultText(fmt.Sprintf("Please use the web portal url: %s to follow the deployment of %s, with the deployment ID of %s", portalURL, project.Name, deployResp.Etag)), nil
	})
}

const DEFANG_PORTAL_HOST = "portal.defang.io"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

// printPlaygroundPortalServiceURLs logs service URLs for the Defang portal
// and returns the first URL for browser preview
func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) string {
	// Log portal URLs for monitoring services
	term.Info("Monitor your services' status in the defang portal")

	// TODO: print all of the urls instead of just the first one.
	// the user may have many publicly accessible services
	// Store the first URL to return for browser preview
	var firstURL string

	for _, serviceInfo := range serviceInfos {
		serviceURL := SERVICE_PORTAL_URL + "/" + serviceInfo.Service.Name
		term.Infof("   - %s", serviceURL)

		// Save the first URL we encounter
		if firstURL == "" {
			firstURL = serviceURL
		}
	}

	return firstURL
}
