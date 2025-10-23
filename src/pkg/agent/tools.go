package agent

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
)

type Connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

type ToolDescriptor struct {
	Name        string
	Description string
}

var (
	LoginTool = ToolDescriptor{
		Name:        "login",
		Description: "Login into Defang",
	}
	ServicesTool = ToolDescriptor{
		Name:        "services",
		Description: "List deployed services for the project in the current working directory",
	}
	DeployTool = ToolDescriptor{
		Name:        "deploy",
		Description: "Deploy the application defined in the docker-compose files in the current working directory",
	}
	DestroyTool = ToolDescriptor{
		Name:        "destroy",
		Description: "Destroy the deployed application defined in the docker-compose files in the current working directory",
	}
	LogsTool = ToolDescriptor{
		Name:        "logs",
		Description: "Fetch logs for the deployed application defined in the docker-compose files in the current working directory",
	}
	EstimateTool = ToolDescriptor{
		Name:        "estimate",
		Description: "Estimate the cost of deployed a Defang project to AWS or GCP",
	}
	SetConfigTool = ToolDescriptor{
		Name:        "set_config",
		Description: "Set a config variable for the defang project",
	}
	RemoveConfigTool = ToolDescriptor{
		Name:        "remove_config",
		Description: "Remove a config variable from the defang project",
	}
	ListConfigsTool = ToolDescriptor{
		Name:        "list_configs",
		Description: "List config variables for the defang project",
	}
	SetAWSProvider = ToolDescriptor{
		Name:        "set_aws_provider",
		Description: "Set the AWS provider for the defang project",
	}
	SetGCPProvider = ToolDescriptor{
		Name:        "set_gcp_provider",
		Description: "Set the GCP provider for the defang project",
	}
	SetPlaygroundProvider = ToolDescriptor{
		Name:        "set_playground_provider",
		Description: "Set the Playground provider for the defang project",
	}
)

type LoginParams struct{}
type ServicesParams struct {
	common.LoaderParams
}
type DeployParams struct {
	common.LoaderParams
}

func CollectTools(cluster string, authPort int, providerId *client.ProviderID) []ai.Tool {
	// loginHandler := MakeLoginToolHandler(cluster, authPort, &LoginCLIAdapter{DefaultToolCLI: &DefaultToolCLI{}})

	return []ai.Tool{
		ai.NewTool[LoginParams, string](
			LoginTool.Name,
			LoginTool.Description,
			func(ctx *ai.ToolContext, _ LoginParams) (string, error) {
				return tools.HandleLoginTool(ctx.Context, cluster, authPort, &tools.LoginCLIAdapter{DefaultToolCLI: &tools.DefaultToolCLI{}})
			},
		),
		ai.NewTool[ServicesParams, string](
			ServicesTool.Name,
			ServicesTool.Description,
			func(ctx *ai.ToolContext, params ServicesParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				var cli tools.CLIInterface = &tools.DefaultToolCLI{}
				return tools.HandleServicesTool(ctx.Context, loader, providerId, cluster, cli)
			},
		),

		ai.NewTool(
			DeployTool.Name,
			DeployTool.Description,
			func(ctx *ai.ToolContext, params DeployParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleDeployTool(ctx.Context, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(DestroyTool.Name,
			DestroyTool.Description,
			func(ctx *ai.ToolContext, params tools.DestroyParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleDestroyTool(ctx.Context, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(LogsTool.Name,
			LogsTool.Description,
			func(ctx *ai.ToolContext, params tools.LogsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleLogsTool(ctx.Context, loader, params, cluster, providerId, cli)
			},
		),
		ai.NewTool(EstimateTool.Name,
			EstimateTool.Description,
			func(ctx *ai.ToolContext, params tools.EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleEstimateTool(ctx.Context, loader, params, cluster, cli)
			},
		),
		ai.NewTool(SetConfigTool.Name,
			SetConfigTool.Description,
			func(ctx *ai.ToolContext, params tools.SetConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleSetConfig(ctx.Context, loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool(RemoveConfigTool.Name,
			RemoveConfigTool.Description,
			func(ctx *ai.ToolContext, params tools.RemoveConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleRemoveConfigTool(ctx.Context, loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool(ListConfigsTool.Name,
			ListConfigsTool.Description,
			func(ctx *ai.ToolContext, params tools.ListConfigsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return tools.HandleListConfigTool(ctx.Context, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(SetAWSProvider.Name,
			SetAWSProvider.Description,
			func(ctx *ai.ToolContext, params tools.SetAWSProviderParams) (string, error) {
				return tools.HandleSetAWSProvider(ctx.Context, params, providerId, cluster)
			},
		),
		ai.NewTool(SetGCPProvider.Name,
			SetGCPProvider.Description,
			func(ctx *ai.ToolContext, params tools.SetGCPProviderParams) (string, error) {
				return tools.HandleSetGCPProvider(ctx.Context, params, providerId, cluster)
			},
		),
		ai.NewTool(SetPlaygroundProvider.Name,
			SetPlaygroundProvider.Description,
			func(ctx *ai.ToolContext, params tools.SetPlaygroundProviderParams) (string, error) {
				return tools.HandleSetPlaygroundProvider(providerId)
			},
		),
	}
}
