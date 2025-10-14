package tools

import (
	"context"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	agentTools "github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/firebase/genkit/go/ai"
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

func translateSchema(schema map[string]any) mcp.ToolInputSchema {
	if schema == nil {
		return mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		}
	}

	schemaType, ok := schema["type"].(string)
	if !ok {
		schemaType = "object"
	}
	schemaProperties, ok := schema["properties"].(map[string]any)
	if !ok {
		schemaProperties = map[string]any{}
	}
	schemaRequired, ok := schema["required"].([]string)
	if !ok {
		schemaRequired = []string{}
	}

	return mcp.ToolInputSchema{
		Type:       schemaType,
		Properties: schemaProperties,
		Required:   schemaRequired,
	}
}

func translateGenKitToolsToMCP(genkitTools []ai.Tool) []server.ServerTool {
	var translatedTools []server.ServerTool
	for _, t := range genkitTools {
		def := t.Definition()
		inputSchema := translateSchema(def.InputSchema)
		translatedTools = append(translatedTools, server.ServerTool{
			Tool: mcp.Tool{
				Name:        t.Name(),
				Description: def.Description,
				InputSchema: inputSchema,
			},
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				result, err := t.RunRaw(ctx, request.GetArguments())
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Tool execution failed", err), nil
				}
				output, ok := result.(string)
				if !ok {
					return mcp.NewToolResultError("Tool returned unexpected result type"), nil
				}
				return mcp.NewToolResultText(output), nil
			},
		})
	}

	return translatedTools
}

func CollectTools(cluster string, authPort int, providerId *client.ProviderID) []server.ServerTool {
	genkitTools := agent.CollectTools(cluster, authPort)
	translatedTools := translateGenKitToolsToMCP(genkitTools)

	return append(translatedTools, []server.ServerTool{
		{
			Tool: mcp.NewTool("services",
				mcp.WithDescription("List deployed services for the project in the current working directory"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				var cli agentTools.CLIInterface = &agentTools.DefaultToolCLI{}
				output, err := agentTools.HandleServicesTool(ctx, loader, providerId, cluster, cli)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to list services", err), err
				}
				return mcp.NewToolResultText(output), nil
			},
		},
		{
			Tool: mcp.NewTool("deploy",
				mcp.WithDescription("Deploy services using defang"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				output, err := agentTools.HandleDeployTool(ctx, loader, providerId, cluster, cli)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to deploy services", err), err
				}
				return mcp.NewToolResultText(output), nil
			},
		},
		{
			Tool: mcp.NewTool("destroy",
				mcp.WithDescription("Destroy deployed services for the project in the current working directory"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				output, err := agentTools.HandleDestroyTool(ctx, loader, providerId, cluster, cli)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to destroy services", err), err
				}
				return mcp.NewToolResultText(output), nil
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
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				params, err := agentTools.ParseLogsParams(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to parse logs parameters", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				output, err := agentTools.HandleLogsTool(ctx, loader, params, cluster, providerId, cli)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to fetch logs", err), err
				}
				return mcp.NewToolResultText(output), nil
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
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				params, err := agentTools.ParseEstimateParams(request, providerId)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to parse estimate parameters", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				output, err := agentTools.HandleEstimateTool(ctx, loader, params, cluster, cli)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to estimate costs", err), err
				}
				return mcp.NewToolResultText(output), nil
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
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				adapter := &agentTools.SetConfigCLIAdapter{DefaultToolCLI: cli}
				params, err := agentTools.ParseSetConfigParams(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to parse set config parameters", err), err
				}
				output, err := agentTools.HandleSetConfig(ctx, loader, params, providerId, cluster, adapter)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to set config", err), err
				}
				return mcp.NewToolResultText(output), nil
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
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				params, err := agentTools.ParseRemoveConfigParams(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to parse remove config parameters", err), err
				}
				output, err := agentTools.HandleRemoveConfigTool(ctx, loader, params, providerId, cluster, &agentTools.RemoveConfigCLIAdapter{DefaultToolCLI: cli})
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to remove config", err), err
				}
				return mcp.NewToolResultText(output), nil
			},
		},
		{
			Tool: mcp.NewTool("list_configs",
				mcp.WithDescription("List config variables for the defang project"),
				workingDirectoryOption,
				multipleComposeFilesOptions,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				loader, err := common.ConfigureLoader(request)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to configure loader", err), err
				}
				cli := &agentTools.DefaultToolCLI{}
				output, err := agentTools.HandleListConfigTool(ctx, loader, providerId, cluster, &agentTools.ListConfigCLIAdapter{DefaultToolCLI: cli})
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to list config", err), err
				}
				return mcp.NewToolResultText(output), nil
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
				output, err := agentTools.HandleSetAWSProvider(ctx, request, providerId, cluster)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to set AWS provider", err), err
				}
				return mcp.NewToolResultText(output), nil
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
				output, err := agentTools.HandleSetGCPProvider(ctx, request, providerId, cluster)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to set GCP provider", err), err
				}
				return mcp.NewToolResultText(output), nil
			},
		},
		{
			Tool: mcp.NewTool("set_playground_provider",
				mcp.WithDescription("Set the Playground provider for the defang project"),
				workingDirectoryOption,
			),
			Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				output, err := agentTools.HandleSetPlaygroundProvider(providerId)
				if err != nil {
					return mcp.NewToolResultErrorFromErr("Failed to set Playground provider", err), err
				}
				return mcp.NewToolResultText(output), nil
			},
		},
	}...)
}
