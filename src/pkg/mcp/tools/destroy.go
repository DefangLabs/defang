package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// setupDestroyTool configures and adds the destroy tool to the MCP server
func setupDestroyTool(s *server.MCPServer, cluster string) {
	term.Info("Creating destroy tool")
	composeDownTool := mcp.NewTool("destroy",
		mcp.WithDescription("Remove services using defang."),

		mcp.WithString("working_directory",
			mcp.Description("Path to current working directory"),
		),
	)
	term.Debug("Destroy tool created")

	// Add the destroy tool handler - make it non-blocking
	term.Info("Adding destroy tool handler")
	s.AddTool(composeDownTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		term.Info("Compose down tool called - removing services")
		track.Evt("MCP Destroy Tool")

		client, err := cli.Connect(ctx, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Could not connect", err), nil
		}

		provider, err := cli.NewProvider(ctx, cliClient.ProviderDefang, client)
		if err != nil {
			term.Error("Failed to get new provider", "error", err)

			return mcp.NewToolResultErrorFromErr("Failed to get new provider", err), nil
		}

		wd, ok := request.Params.Arguments["working_directory"].(string)
		if !ok || wd != "" {
			err := os.Chdir(wd)
			if err != nil {
				term.Error("Failed to change working directory", "error", err)
			}
		}

		loader := configureLoader(request)

		projectName, err := cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			term.Error("Failed to load project name", "error", err)
			return mcp.NewToolResultErrorFromErr("Failed to load project name", err), nil
		}

		err = canIUseProvider(ctx, client, projectName, provider)
		if err != nil {
			term.Error("Failed to use provider", "error", err)
			return mcp.NewToolResultErrorFromErr("Failed to use provider", err), nil
		}

		deployment, err := cli.ComposeDown(ctx, projectName, client, provider)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				term.Warn("Project not found", "error", err)
				return mcp.NewToolResultText("Project not found, nothing to destroy. Please use a valid project name, compose file path or project directory."), nil
			}

			result := HandleTermsOfServiceError(err)
			if result != nil {
				return result, nil
			}

			return mcp.NewToolResultErrorFromErr("Failed to destroy project", err), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully destroyed project: %s, etag: %s", projectName, deployment)), nil
	})
}

func canIUseProvider(ctx context.Context, grpcClient cliClient.FabricClient, projectName string, provider cliClient.Provider) error {
	canUseReq := defangv1.CanIUseRequest{
		Project:  projectName,
		Provider: defangv1.Provider_DEFANG,
	}

	resp, err := grpcClient.CanIUse(ctx, &canUseReq)
	if err != nil {
		term.Error("Failed to use provider", "error", err)
		return fmt.Errorf("failed to use provider: %w", err)
	}

	provider.SetCanIUseConfig(resp)
	return nil
}
