package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupEstimateTool configures and adds the estimate tool to the MCP server
func setupEstimateTool(s *server.MCPServer, cluster string, region string) {
	term.Debug("Creating estimate tool")
	loginTool := mcp.NewTool("estimate",
		mcp.WithDescription("Estimate the cost of a Defang project deployed to AWS"),
	)
	term.Debug("Login tool created")

	// Add the Estimate tool handler - make it non-blocking
	term.Debug("Adding estimate tool handler")
	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Debug("Estimate tool called")
		// Test token
		term.Debug("Function invoked: cli.Connect")
		track.Evt("MCP Estimate Tool")

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

		providerID := cliClient.ProviderAWS // Default to AWS

		estimate, err := cli.RunEstimate(ctx, project, client, providerID, region, defangv1.DeploymentMode_PRODUCTION)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to run estimate", err), nil
		}
		term.Debugf("Estimate: %+v", estimate)

		return mcp.NewToolResultText(fmt.Sprintf("Successfully estimated the cost of the project to AWS: %+v", estimate)), nil
	})
}
