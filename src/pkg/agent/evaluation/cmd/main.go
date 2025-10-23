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

// TODO: Should match CollectTools.
func CollectMockTools(cluster string, authPort int, providerId *client.ProviderID) []ai.Tool {
	return []ai.Tool{
		ai.NewTool[agent.LoginParams, string](
			agent.LoginTool.Name,
			agent.LoginTool.Description,
			func(ctx *ai.ToolContext, params agent.LoginParams) (string, error) {
				return dummyToolHandler(ctx, agent.LoginTool.Name, params)
			},
		),
		ai.NewTool[agent.ServicesParams, string](
			agent.ServicesTool.Name,
			agent.ServicesTool.Description,
			func(ctx *ai.ToolContext, params agent.ServicesParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				var cli tools.CLIInterface = &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.ServicesTool.Name, loader, providerId, cluster, cli)
			},
		),

		ai.NewTool(agent.DeployTool.Name,
			agent.DeployTool.Description,
			func(ctx *ai.ToolContext, params agent.DeployParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.DeployTool.Name, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(agent.DestroyTool.Name,
			agent.DestroyTool.Description,
			func(ctx *ai.ToolContext, params tools.DestroyParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.DestroyTool.Name, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(agent.LogsTool.Name,
			agent.LogsTool.Description,
			func(ctx *ai.ToolContext, params tools.LogsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.LogsTool.Name, loader, params, cluster, providerId, cli)
			},
		),
		ai.NewTool(agent.EstimateTool.Name,
			agent.EstimateTool.Description,
			func(ctx *ai.ToolContext, params tools.EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.EstimateTool.Name, loader, params, cluster, cli)
			},
		),
		ai.NewTool(agent.SetConfigTool.Name,
			agent.SetConfigTool.Description,
			func(ctx *ai.ToolContext, params tools.SetConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.SetConfigTool.Name, loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool(agent.RemoveConfigTool.Name,
			agent.RemoveConfigTool.Description,
			func(ctx *ai.ToolContext, params tools.RemoveConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.RemoveConfigTool.Name, loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool(agent.ListConfigsTool.Name,
			agent.ListConfigsTool.Description,
			func(ctx *ai.ToolContext, params tools.ListConfigsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, agent.ListConfigsTool.Name, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(agent.SetAWSProvider.Name,
			agent.SetAWSProvider.Description,
			func(ctx *ai.ToolContext, params tools.SetAWSProviderParams) (string, error) {
				return dummyToolHandler(ctx, agent.SetAWSProvider.Name, params, providerId, cluster)
			},
		),
		ai.NewTool(agent.SetGCPProvider.Name,
			agent.SetGCPProvider.Description,
			func(ctx *ai.ToolContext, params tools.SetGCPProviderParams) (string, error) {
				return dummyToolHandler(ctx, agent.SetGCPProvider.Name, params, providerId, cluster)
			},
		),
		ai.NewTool(agent.SetPlaygroundProvider.Name,
			agent.SetPlaygroundProvider.Description,
			func(ctx *ai.ToolContext, params tools.SetPlaygroundProviderParams) (string, error) {
				return dummyToolHandler(ctx, agent.SetPlaygroundProvider.Name, params, providerId, cluster)
			},
		),
	}
}
