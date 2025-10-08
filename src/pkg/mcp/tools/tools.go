package tools

import (
	"context"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var workingDirectoryOption = mcp.WithString("working_directory",
	mcp.Description("Path to project's working directory"),
	mcp.Required(),
)

var multipleComposeFilesOptions = mcp.WithArray("compose_file_paths",
	mcp.Description("Path(s) to docker-compose files"),
	mcp.Items(map[string]string{"type": "string"}),
)

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
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var cli CLIInterface = &DefaultToolCLI{}
				return handleServicesTool(ctx, request, providerId, cluster, cli)
			},
		},
		{
			Tool: mcp.NewTool("deploy",
				mcp.WithDescription("Deploy services using defang"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleDeployTool(ctx, request, providerId, cluster, cli)
			},
		},
		{
			Tool: mcp.NewTool("destroy",
				mcp.WithDescription("Destroy deployed services for the project in the current working directory"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleDestroyTool(ctx, request, providerId, cluster, cli)
			},
		},
		{
			Tool: mcp.NewTool("logs",
				mcp.WithDescription("Fetch logs for a deployment."),
				workingDirectoryOption,
				mcp.WithString("deployment_id",
					mcp.Description("The deployment ID for which to fetch logs"),
				),
				mcp.WithString("since",
					mcp.Description("The start time in RFC3339 format (e.g., 2006-01-02T15:04:05Z07:00)"),
					mcp.Required(),
					mcp.DefaultString(time.Now().Add(-1*time.Hour).Format(time.RFC3339)),
				),
				mcp.WithString("until",
					mcp.Description("The end time in RFC3339 format (e.g., 2006-01-02T15:04:05Z07:00)"),
					mcp.Required(),
					mcp.DefaultString(time.Now().Format(time.RFC3339)),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleLogsTool(ctx, request, cluster, providerId, cli)
			},
		},
		{
			Tool: mcp.NewTool("estimate",
				mcp.WithDescription("Estimate the cost of deployed a Defang project."),
				workingDirectoryOption,
				multipleComposeFilesOptions,
				mcp.WithString("provider",
					mcp.Description("The cloud provider to estimate costs for. Supported options are AWS or GCP"),
					mcp.DefaultString(strings.ToUpper(providerId.String())),
					mcp.Enum("AWS", "GCP"),
				),
				mcp.WithString("deployment_mode",
					mcp.Description("The deployment mode for the estimate. Options are: "+strings.Join(modes.AllDeploymentModes(), ", ")),
					mcp.DefaultString("AFFORDABLE"),
					mcp.Enum(modes.AllDeploymentModes()...),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleEstimateTool(ctx, request, providerId, cluster, cli)
			},
		},
		{
			Tool: mcp.NewTool("set_config",
				mcp.WithDescription("Tail logs for a deployment."),
				workingDirectoryOption,
				multipleComposeFilesOptions,
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
				return handleSetConfig(ctx, request, providerId, cluster, adapter)
			},
		},
		{
			Tool: mcp.NewTool("remove_config",
				mcp.WithDescription("Remove a config variable from the defang project"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
				mcp.WithString("key",
					mcp.Description("The config key to remove"),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleRemoveConfigTool(ctx, request, providerId, cluster, &RemoveConfigCLIAdapter{DefaultToolCLI: cli})
			},
		},
		{
			Tool: mcp.NewTool("list_configs",
				mcp.WithDescription("List config variables for the defang project"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				cli := &DefaultToolCLI{}
				return handleListConfigTool(ctx, request, providerId, cluster, &ListConfigCLIAdapter{DefaultToolCLI: cli})
			},
		},
		{
			Tool: mcp.NewTool("set_aws_provider",
				mcp.WithDescription("Set the AWS provider for the defang project"),
				workingDirectoryOption,
				mcp.WithString("accessKeyId",
					mcp.Description("Your AWS Access Key ID"),
				),
				mcp.WithString("secretAccessKey",
					mcp.Description("Your AWS Secret Access Key"),
				),
				mcp.WithString("region",
					mcp.Description("Your AWS Region"),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return handleSetAWSProvider(ctx, request, providerId, cluster)
			},
		},
		{
			Tool: mcp.NewTool("set_gcp_provider",
				mcp.WithDescription("Set the GCP provider for the defang project"),
				workingDirectoryOption,
				mcp.WithString("gcpProjectId",
					mcp.Description("Your GCP Project ID"),
				),
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return handleSetGCPProvider(ctx, request, providerId, cluster)
			},
		},
		{
			Tool: mcp.NewTool("set_playground_provider",
				mcp.WithDescription("Set the Playground provider for the defang project"),
				workingDirectoryOption,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return handleSetPlaygroundProvider(providerId)
			},
		},
	}
	return tools
}
