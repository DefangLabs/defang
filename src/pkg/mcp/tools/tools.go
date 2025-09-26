package tools

import (
	"context"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var workingDirectoryOption = mcp.WithString("working_directory",
	mcp.Description("Path to project's working directory"),
)

// SetupTools configures and adds all the MCP tools to the server
func CollectTools(cluster string, authPort int, providerId *client.ProviderID) []server.ServerTool {
	tools := []server.ServerTool{
		{
			Tool: mcp.NewTool("login",
				mcp.WithDescription("Login to Defang"),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleLoginTool(ctx, request, cluster, authPort, &LoginCLIAdapter{DefaultToolCLI: cli})
			},
		},
		{
			Tool: mcp.NewTool("services",
				mcp.WithDescription("List deployed services for the project in the current working directory"),
				workingDirectoryOption,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultCLI{}
				deploymentInfo := &DefaultDeploymentInfo{}
				track.Evt("MCP Services Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleServicesTool(ctx, request, providerId, cluster, cli, deploymentInfo)
				track.Evt("MCP Services Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
		{
			Tool: mcp.NewTool("deploy",
				mcp.WithDescription("Deploy services using defang"),
				workingDirectoryOption,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				track.Evt("MCP Deploy Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleDeployTool(ctx, request, providerId, cluster, cli)
				track.Evt("MCP Deploy Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
		{
			Tool: mcp.NewTool("destroy",
				mcp.WithDescription("Destroy deployed services for the project in the current working directory"),
				workingDirectoryOption,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				track.Evt("MCP Destroy Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleDestroyTool(ctx, request, providerId, cluster, cli)
				track.Evt("MCP Destroy Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
		{
			Tool: mcp.NewTool("estimate",
				mcp.WithDescription("Estimate the cost of deployed a Defang project."),
				workingDirectoryOption,
				mcp.WithString("provider",
					mcp.Description("The cloud provider to estimate costs for. Supported options are AWS or GCP"),
					mcp.DefaultString(strings.ToUpper(providerId.String())),
					mcp.Enum("AWS", "GCP"),
				),

				mcp.WithString("deployment_mode",
					mcp.Description("The deployment mode for the estimate. Options are AFFORDABLE, BALANCED or HIGH AVAILABILITY."),
					mcp.DefaultString("AFFORDABLE"),
					mcp.Enum("AFFORDABLE", "BALANCED", "HIGH AVAILABILITY"),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultEstimateCLI{}
				track.Evt("MCP Estimate Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleEstimateTool(ctx, request, providerId, cluster, cli)
				track.Evt("MCP Estimate Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
		{
			Tool: mcp.NewTool("set_config",
				mcp.WithDescription("Tail logs for a deployment."),
				workingDirectoryOption,
				mcp.WithString("key",
					mcp.Description("The config key to set"),
				),
				mcp.WithString("value",
					mcp.Description("The config value to set"),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				adapter := &SetConfigCLIAdapter{DefaultToolCLI: cli}
				track.Evt("MCP Set Config Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleSetConfig(ctx, request, cluster, providerId, adapter)
				track.Evt("MCP Set Config Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
		{
			Tool: mcp.NewTool("remove_config",
				mcp.WithDescription("Remove a config variable from the defang project"),
				workingDirectoryOption,
				mcp.WithString("key",
					mcp.Description("The config key to remove"),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				track.Evt("MCP Remove Config Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleRemoveConfigTool(ctx, request, providerId, cluster, &RemoveConfigCLIAdapter{DefaultToolCLI: cli})
				track.Evt("MCP Remove Config Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
		{
			Tool: mcp.NewTool("list_configs",
				mcp.WithDescription("List config variables for the defang project"),
				workingDirectoryOption,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				track.Evt("MCP List Config Tool", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient))
				resp, err := handleListConfigTool(ctx, request, providerId, cluster, &ListConfigCLIAdapter{DefaultToolCLI: cli})
				track.Evt("MCP List Config Tool Done", track.P("provider", *providerId), track.P("cluster", cluster), track.P("client", MCPDevelopmentClient), track.P("error", err))
				return resp, err
			},
		},
	}
	return tools
}
