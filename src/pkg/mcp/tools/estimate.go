package tools

import (
	"bytes"
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

// setupEstimateTool configures and adds the estimate tool to the MCP server
func setupEstimateTool(s *server.MCPServer, cluster string) {
	term.Debug("Creating estimate tool")
	estimateTool := mcp.NewTool("estimate",
		mcp.WithDescription("Estimate the cost of a Defang project deployed to AWS"),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),

		mcp.WithString("deployment_mode",
			mcp.Description("The deployment mode for the estimate. Options are AFFORDABLE, BALANCED or HIGH AVAILABILITY."),
			mcp.DefaultString("AFFORDABLE"),
			mcp.Enum("AFFORDABLE", "BALANCED", "HIGH AVAILABILITY"),
		),
	)
	term.Debug("Estimate tool created")

	// Add the Estimate tool handler
	term.Debug("Adding estimate tool handler")
	s.AddTool(estimateTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Debug("Estimate tool called")
		track.Evt("MCP Estimate Tool")

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if ok && wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
		}

		modeString, ok := request.Params.Arguments["deployment_mode"].(string)
		if !ok {
			modeString = "AFFORDABLE" // Default to AFFORDABLE if not provided
		}

		// This logic is replicated from src/cmd/cli/command/mode.go
		// I couldn't figure out how to import it without circular dependencies
		modeString = strings.ToUpper(modeString)
		var mode defangv1.DeploymentMode
		switch modeString {
		case "AFFORDABLE":
			mode = defangv1.DeploymentMode_DEVELOPMENT
		case "BALANCED":
			mode = defangv1.DeploymentMode_STAGING
		case "HIGH AVAILABILITY":
			mode = defangv1.DeploymentMode_PRODUCTION
		default:
			term.Warn("Unknown deployment mode provided, defaulting to AFFORDABLE")
			mode = defangv1.DeploymentMode_DEVELOPMENT
		}

		term.Debugf("Deployment mode set to: %s", mode.String())

		loader := configureLoader(request)

		term.Debug("Function invoked: loader.LoadProject")
		project, err := loader.LoadProject(ctx)
		if err != nil {
			err = fmt.Errorf("failed to parse compose file: %w", err)
			term.Error("Failed to deploy services", "error", err)

			return mcp.NewToolResultText(fmt.Sprintf("Estimate failed: %v. Please provide a valid compose file path.", err)), nil
		}

		term.Debug("Function invoked: cli.Connect")
		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Could not connect", err), nil
		}

		defangProvider := &cliClient.PlaygroundProvider{FabricClient: client}
		providerID := cliClient.ProviderAWS // Default to AWS

		term.Debug("Function invoked: cli.RunEstimate")
		estimate, err := cli.RunEstimate(ctx, project, client, defangProvider, providerID, "us-west-2", mode)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to run estimate", err), nil
		}
		term.Debugf("Estimate: %+v", estimate)

		oldTerm := term.DefaultTerm
		stdout := new(bytes.Buffer)
		term.DefaultTerm = term.NewTerm(
			os.Stdin,
			stdout,
			new(bytes.Buffer),
		)

		cli.PrintEstimate(mode, estimate)

		term.DefaultTerm = oldTerm

		estimateText := stdout.String()

		return mcp.NewToolResultText("Successfully estimated the cost of the project to AWS:\n" + estimateText), nil
	})
}
