package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/auth"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupServicesTool configures and adds the services tool to the MCP server
func setupServicesTool(s *server.MCPServer) {
	term.Info("Creating services tool")
	servicesTool := mcp.NewTool("services",
		mcp.WithDescription("List information about services in Defang"),
		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("Services tool created")

	// Add the services tool handler - make it non-blocking
	term.Info("Adding services tool handler")
	s.AddTool(servicesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get services
		term.Info("Services tool called - fetching services from Defang")

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if ok && wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
		}

		loader := configureLoader(request)

		token := auth.GetExistingToken()

		// Create a Defang client
		grpcClient := client.NewGrpcClient(auth.Host, token, types.TenantName(""))
		provider, err := cli.NewProvider(ctx, client.ProviderDefang, grpcClient)
		if err != nil {
			term.Error("Failed to create provider", "error", err)
			return mcp.NewToolResultText(fmt.Sprintf("Failed to create provider: %v", err)), nil
		}

		projectName, err := client.LoadProjectNameWithFallback(ctx, loader, provider)
		term.Info("Project name loaded", "project", projectName)
		if err != nil {
			if strings.Contains(err.Error(), "no projects found") {
				term.Errorf("No projects found on Playground, error: %v", err)
				return mcp.NewToolResultText("No projects found on Playground"), nil
			}
			term.Errorf("Failed to load project name, error: %v", err)
			return mcp.NewToolResultText(fmt.Sprintf("Failed to load project name: %v", err)), nil
		}

		serviceResponse, err := deployment_info.GetServices(ctx, projectName, provider)
		if err != nil {
			var noServicesErr cli.ErrNoServices
			if errors.As(err, &noServicesErr) {
				term.Warnf("No services found for the specified project %s", projectName)
				return mcp.NewToolResultText("No services found for the specified project " + projectName), nil
			}
			if connect.CodeOf(err) == connect.CodeNotFound && strings.Contains(err.Error(), "is not deployed in Playground") {
				term.Warnf("Project %s is not deployed in Playground", projectName)
				return mcp.NewToolResultText(fmt.Sprintf("Project %s is not deployed in Playground", projectName)), nil
			}
			term.Error("Failed to get services", "error", err)
			return mcp.NewToolResultText("Failed to get services"), nil
		}

		// Convert to JSON
		jsonData, jsonErr := json.Marshal(serviceResponse)
		if jsonErr == nil {
			term.Info("Successfully loaded services", "count", len(serviceResponse), "data", string(jsonData))
			// Use NewToolResultText with JSON string
			return mcp.NewToolResultText(string(jsonData)), nil
		}

		// Return the data in a structured format
		return mcp.NewToolResultText("Successfully loaded services, but failed to convert to JSON. Please check the logs for details."), nil
	})
}
