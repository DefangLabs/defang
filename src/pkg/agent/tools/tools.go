package tools

import (
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/firebase/genkit/go/ai"
)

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
				return HandleServicesTool(ctx.Context, loader, cli, ec, &config)
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
				return HandleDeployTool(ctx.Context, loader, cli, ec, &config)
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
				return HandleDestroyTool(ctx.Context, loader, cli, ec, &config)
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
				return HandleLogsTool(ctx.Context, loader, params, cli, ec, &config)
			},
		),
		ai.NewTool("estimate",
			"Estimate the cost of deploying a Defang project to AWS or GCP",
			func(ctx *ai.ToolContext, params EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleEstimateTool(ctx.Context, loader, params, cli, &config)
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
				return HandleSetConfig(ctx.Context, loader, params, cli, ec, &config)
			},
		),
		ai.NewTool("select_stack",
			"Select the deployment stack for the defang project",
			func(ctx *ai.ToolContext, params SelectStackParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleSelectStackTool(ctx.Context, loader, cli, params, &config)
			},
		),
		ai.NewTool("create_aws_stack",
			"Create a defang stack file to deploy to AWS",
			func(ctx *ai.ToolContext, params CreateAWSStackParams) (string, error) {
				return HandleCreateAWSStackTool(ctx.Context, params, &config)
			},
		),
		ai.NewTool("create_gcp_stack",
			"Create a defang stack file to deploy to GCP",
			func(ctx *ai.ToolContext, params CreateGCPStackParams) (string, error) {
				return HandleCreateGCPStackTool(ctx.Context, params, &config)
			},
		),
		ai.NewTool("current_stack",
			"Get the currently selected stack",
			func(ctx *ai.ToolContext, params struct{}) (string, error) {
				return HandleCurrentStackTool(ctx.Context, &config)
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
				return HandleRemoveConfigTool(ctx.Context, loader, params, cli, ec, &config)
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
				return HandleListConfigTool(ctx.Context, loader, cli, ec, &config)
			},
		),
	}
}
