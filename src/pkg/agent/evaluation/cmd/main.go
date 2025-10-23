package main

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/evaluators"
)

func main() {
	ctx := context.Background()
	provider := client.ProviderAWS

	config := agent.AgentConfig{
		Cluster:        "cluster",
		AuthPort:       8080,
		ProviderId:     &provider,
		EvaluationMode: true,
		EvalMetrics: []evaluators.MetricConfig{
			{MetricType: evaluators.EvaluatorRegex},
			{MetricType: evaluators.EvaluatorDeepEqual},
		},
	}

	tools := CollectMockTools(config.Cluster, config.AuthPort, config.ProviderId)
	a := agent.NewWithEvaluation(ctx, config, tools)
	a.CreateEvaluationFlow()

	// keep the main function running fo developer UI testing
	select {}
}

func dummyToolHandler(ctx *ai.ToolContext, name string, args ...any) (string, error) {
	return fmt.Sprintf("This is a dummy tool response for %s with args: %v", name, args), nil
}

func CollectMockTools(cluster string, authPort int, providerId *client.ProviderID) []ai.Tool {
	return []ai.Tool{
		ai.NewTool[agent.LoginParams, string](
			"login",
			"Login into Defang",
			func(ctx *ai.ToolContext, _ agent.LoginParams) (string, error) {
				return dummyToolHandler(ctx, "login")
			},
		),
		ai.NewTool[agent.ServicesParams, string](
			"services",
			"List deployed services for the project in the current working directory",
			func(ctx *ai.ToolContext, params agent.ServicesParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				var cli tools.CLIInterface = &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "services", loader, providerId, cluster, cli)
			},
		),

		ai.NewTool("deploy",
			"Deploy the application defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params agent.DeployParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "deploy", loader, providerId, cluster, cli)
			},
		),
		ai.NewTool("destroy",
			"Destroy the deployed application defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params tools.DestroyParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "destroy", loader, providerId, cluster, cli)
			},
		),
		ai.NewTool("logs",
			"Fetch logs for the deployed application defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params tools.LogsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "logs", loader, params, cluster, providerId, cli)
			},
		),
		ai.NewTool("estimate",
			"Estimate the cost of deployed a Defang project to AWS or GCP",
			func(ctx *ai.ToolContext, params tools.EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "estimate", loader, params, cluster, cli)
			},
		),
		ai.NewTool("set_config",
			"Set a config variable for the defang project",
			func(ctx *ai.ToolContext, params tools.SetConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "set_config", loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool("remove_config",
			"Remove a config variable from the defang project",
			func(ctx *ai.ToolContext, params tools.RemoveConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "remove_config", loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool("list_configs",
			"List config variables for the defang project",
			func(ctx *ai.ToolContext, params tools.ListConfigsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, "list_configs", loader, providerId, cluster, cli)
			},
		),
		ai.NewTool("set_aws_provider",
			"Set the AWS provider for the defang project",
			func(ctx *ai.ToolContext, params tools.SetAWSProviderParams) (string, error) {
				return dummyToolHandler(ctx, "set_aws_provider", params, providerId, cluster)
			},
		),
		ai.NewTool("set_gcp_provider",
			"Set the GCP provider for the defang project",
			func(ctx *ai.ToolContext, params tools.SetGCPProviderParams) (string, error) {
				return dummyToolHandler(ctx, "set_gcp_provider", params, providerId, cluster)
			},
		),
		ai.NewTool("set_playground_provider",
			"Set the Playground provider for the defang project",
			func(ctx *ai.ToolContext, params tools.SetPlaygroundProviderParams) (string, error) {
				return dummyToolHandler(ctx, "set_playground_provider", params, providerId, cluster)
			},
		),
	}
}
