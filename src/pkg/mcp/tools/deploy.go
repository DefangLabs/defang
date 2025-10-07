package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

func handleDeployTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli DeployCLIInterface) (*mcp.CallToolResult, error) {
	err := common.ProviderNotConfiguredError(*providerId)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("No provider configured", err), err
	}

	// Get compose path
	term.Debug("Compose up tool called - deploying services")

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

		err = common.FixupConfigError(err)
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
			err := cli.OpenBrowser(portalURL)
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
