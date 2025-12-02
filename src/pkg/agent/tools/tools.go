package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/firebase/genkit/go/ai"
)

type Connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

func CollectDefangTools(ec elicitations.Controller, config StackConfig) []ai.Tool {
	return []ai.Tool{
		ai.NewTool[ServicesParams, string](
			"services",
			"List deployed services for the project in the current working directory",
			func(ctx *ai.ToolContext, params ServicesParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				var cli CLIInterface = &DefaultToolCLI{}
				return HandleServicesTool(ctx, loader, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("deploy",
			"Initiate deployment of the application defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params DeployParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleDeployTool(ctx, loader, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("destroy",
			"Destroy the deployed application defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params DestroyParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleDestroyTool(ctx, loader, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("logs",
			"Fetch logs for the application in pages of up to 100 lines. You can use the 'since' and 'until' parameters to page through logs by time.",
			func(ctx *ai.ToolContext, params LogsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleLogsTool(ctx, loader, params, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("estimate",
			"Estimate the cost of deployed a Defang project to AWS or GCP",
			func(ctx *ai.ToolContext, params EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleEstimateTool(ctx, loader, params, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("set_config",
			"Set a config variable for the defang project",
			func(ctx *ai.ToolContext, params SetConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleSetConfig(ctx, loader, params, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("remove_config",
			"Remove a config variable from the defang project",
			func(ctx *ai.ToolContext, params RemoveConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleRemoveConfigTool(ctx, loader, params, config.ProviderId, config.Cluster, cli)
			},
		),
		ai.NewTool("list_configs",
			"List config variables for the defang project",
			func(ctx *ai.ToolContext, params ListConfigsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleListConfigTool(ctx, loader, config.ProviderId, config.Cluster, cli)
			},
		),
	}
}
