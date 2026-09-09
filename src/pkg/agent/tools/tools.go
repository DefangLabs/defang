package tools

import (
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/firebase/genkit/go/ai"
)

func CollectDefangTools(ec elicitations.Controller, sc StackConfig) []ai.Tool {
	return []ai.Tool{
		ai.NewTool(
			"services",
			"List deployed services for the selected project stack in the current working directory",
			func(ctx *ai.ToolContext, params ServicesParams) (string, error) {
				var cli CLIInterface = &DefaultToolCLI{}
				return HandleServicesTool(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("deploy",
			"Initiate deployment of the application stack defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params DeployParams) (string, error) {
				cli := &DefaultToolCLI{}
				return HandleDeployTool(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("destroy",
			"Destroy the deployed application in the selected stack, defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params DestroyParams) (string, error) {
				cli := &DefaultToolCLI{}
				return HandleDestroyTool(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("cleanup_resources",
			"Find and remove cloud resources left behind after `defang down` (on AWS: load balancers and databases with deletion protection, leftover Route53 records, and non-empty ECR repositories; on Azure: the project's resource group and the Key Vault inside it). Performs the minimum action needed for each (disable deletion protection, delete records, delete images, delete the resource group) and confirms before each change. After running, run `defang down` again so Pulumi can finish removing any AWS resources it was blocked from deleting.",
			func(ctx *ai.ToolContext, params CleanupParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams, sc.Stack)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleCleanupTool(ctx.Context, loader, params, cli, ec, sc)
			},
		),
		ai.NewTool("logs",
			"Fetch logs for the application in the selected stack, in pages of up to 100 lines. You can use the 'since' and 'until' parameters to page through logs by time.",
			func(ctx *ai.ToolContext, params LogsParams) (string, error) {
				cli := &DefaultToolCLI{}
				return HandleLogsTool(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("estimate",
			"Estimate the cost of deploying a Defang project to AWS or GCP",
			func(ctx *ai.ToolContext, params EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams, sc.Stack)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &DefaultToolCLI{}
				return HandleEstimateTool(ctx.Context, loader, params, cli, sc)
			},
		),
		ai.NewTool("set_config",
			"Set a config variable for the currently selected stack in the Defang project",
			func(ctx *ai.ToolContext, params SetConfigParams) (string, error) {
				cli := &DefaultToolCLI{}
				return HandleSetConfig(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("select_stack",
			"Select the deployment stack for the Defang project",
			func(ctx *ai.ToolContext, params SelectStackParams) (string, error) {
				return HandleSelectStackTool(ctx.Context, params, sc)
			},
		),
		ai.NewTool("create_aws_stack",
			"Create a defang stack file to deploy to AWS",
			func(ctx *ai.ToolContext, params CreateAWSStackParams) (string, error) {
				return HandleCreateAWSStackTool(ctx.Context, params, sc)
			},
		),
		ai.NewTool("create_azure_stack",
			"Create a defang stack file to deploy to Azure",
			func(ctx *ai.ToolContext, params CreateAzureStackParams) (string, error) {
				return HandleCreateAzureStackTool(ctx.Context, params, sc)
			},
		),
		ai.NewTool("create_gcp_stack",
			"Create a defang stack file to deploy to GCP",
			func(ctx *ai.ToolContext, params CreateGCPStackParams) (string, error) {
				return HandleCreateGCPStackTool(ctx.Context, params, sc)
			},
		),
		ai.NewTool("list_stacks",
			"List all the Defang stack(s) for the current project",
			func(ctx *ai.ToolContext, params ListStacksParams) (string, error) {
				return HandleListStacksTool(ctx.Context, params)
			},
		),
		ai.NewTool("current_stack",
			"Get the currently selected stack",
			func(ctx *ai.ToolContext, params CurrentStackParams) (string, error) {
				return HandleCurrentStackTool(ctx.Context, sc)
			},
		),
		ai.NewTool("remove_config",
			"Remove a config variable from the currently selected stack in the Defang project",
			func(ctx *ai.ToolContext, params RemoveConfigParams) (string, error) {
				cli := &DefaultToolCLI{}
				return HandleRemoveConfigTool(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("list_configs",
			"List config variables for the currently selected stack in the Defang project",
			func(ctx *ai.ToolContext, params ListConfigParams) (string, error) {
				cli := &DefaultToolCLI{}
				return HandleListConfigTool(ctx.Context, params, cli, ec, sc)
			},
		),
		ai.NewTool("project_name",
			"Get the current project name",
			func(ctx *ai.ToolContext, params ProjectNameParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams, sc.Stack)
				if err != nil {
					return "Failed to configure loader", err
				}
				return HandleProjectNameTool(ctx, loader)
			},
		),
	}
}
