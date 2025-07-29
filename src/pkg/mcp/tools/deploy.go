package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

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

// setupDeployTool configures and adds the deployment tool to the MCP server
func setupDeployTool(s *server.MCPServer, cluster string, providerId cliClient.ProviderID) {
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
		// Get compose path
		term.Debug("Compose up tool called - deploying services")
		track.Evt("MCP Deploy Tool")

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if ok && wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
		}

		loader := configureLoader(request)

		term.Debug("Function invoked: loader.LoadProject")
		project, err := loader.LoadProject(ctx)
		if err != nil {
			err = fmt.Errorf("failed to parse compose file: %w", err)
			term.Error("Failed to deploy services", "error", err)

			return mcp.NewToolResultText(fmt.Sprintf("Local deployment failed: %v. Please provide a valid compose file path.", err)), nil
		}

		term.Debug("Function invoked: cli.Connect")
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Could not connect", err), nil
		}

		client.Track("MCP Deploy Tool")

		term.Debug("Function invoked: cli.NewProvider")
		provider, err := cli.NewProvider(ctx, providerId, client)
		if err != nil {
			term.Error("Failed to get new provider", "error", err)

			return mcp.NewToolResultErrorFromErr("Failed to get new provider", err), nil
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
				return result, nil
			}
			result = HandleConfigError(err)
			if result != nil {
				return result, nil
			}

			return mcp.NewToolResultErrorFromErr("Failed to compose up services", err), nil
		}

		if len(deployResp.Services) == 0 {
			term.Error("Failed to deploy services", "error", errors.New("no services deployed"))
			return mcp.NewToolResultText(fmt.Sprintf("Failed to deploy services: %v", errors.New("no services deployed"))), nil
		}

		// Get the portal URL for browser preview
		portalURL := "https://portal.defang.io/"

		// Open the portal URL in the browser
		term.Debugf("Opening portal URL in browser: %s", portalURL)
		go func() {
			err := browser.OpenURL(portalURL)
			if err != nil {
				term.Error("Failed to open URL in browser", "error", err, "url", portalURL)
			}
		}()

		// Success case
		term.Debugf("Successfully started deployed services with etag: %s", deployResp.Etag)

		// Log deployment success
		term.Debug("Deployment Started!")
		term.Debugf("Deployment ID: %s", deployResp.Etag)

		// Log browser preview information
		term.Debugf("🌐 %s available", portalURL)

		// Log service details
		term.Debug("Services:")
		for _, serviceInfo := range deployResp.Services {
			term.Debugf("- %s", serviceInfo.Service.Name)
			term.Debugf("  Public URL: %s", serviceInfo.PublicFqdn)
			term.Debugf("  Status: %s", serviceInfo.Status)
		}

		// Return the etag data as text
		return mcp.NewToolResultText(fmt.Sprintf("Please use the web portal url: %s to follow the deployment of %s, with the deployment ID of %s", portalURL, project.Name, deployResp.Etag)), nil
	})
}
