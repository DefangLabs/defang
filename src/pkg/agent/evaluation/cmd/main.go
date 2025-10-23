package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/evaluators"
)

type EvalConfiguration struct {
	AIModel string                  `json:"aiModel,omitempty"`
	Tools   map[string]ToolOverride `json:"tools,omitempty"`
}

type ToolOverride struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

func main() {
	var configFile = flag.String("config", "", "Path to JSON configuration file")
	flag.Parse()

	evalConfig := generateEvaluationConfig(configFile)

	ctx := context.Background()
	provider := client.ProviderAWS

	config := agent.AgentConfig{
		Cluster:        "cluster",
		AuthPort:       8080,
		ProviderId:     &provider,
		AIModel:        evalConfig.AIModel,
		EvaluationMode: true,
		EvalMetrics: []evaluators.MetricConfig{
			{MetricType: evaluators.EvaluatorRegex},
			{MetricType: evaluators.EvaluatorDeepEqual},
		},
	}

	tools := CollectMockToolsWithConfig(config.Cluster, config.AuthPort, config.ProviderId, evalConfig.Tools)
	a := agent.NewWithEvaluation(ctx, config, tools)
	a.CreateEvaluationFlow()

	// keep the main function running fo developer UI testing
	select {}
}

func generateEvaluationConfig(configFile *string) EvalConfiguration {
	var evalConfig = EvalConfiguration{
		AIModel: "googleai/gemini-2.5-flash", // Default model
		Tools:   make(map[string]ToolOverride),
	}

	// Load configuration from JSON file if provided
	if *configFile != "" {
		if data, err := os.ReadFile(*configFile); err == nil {
			var fileConfig EvalConfiguration
			if err := json.Unmarshal(data, &fileConfig); err == nil {
				if fileConfig.AIModel != "" {
					evalConfig.AIModel = fileConfig.AIModel
				}
				if fileConfig.Tools != nil {
					evalConfig.Tools = fileConfig.Tools
				}
			} else {
				fmt.Printf("Warning: Failed to parse config file %s: %v\n", *configFile, err)
			}
		} else {
			fmt.Printf("Warning: Could not read config file %s: %v\n", *configFile, err)
		}
	}
	return evalConfig
}

func dummyToolHandler(ctx *ai.ToolContext, name string, args ...any) (string, error) {
	return fmt.Sprintf("This is a dummy tool response for %s with args: %v", name, args), nil
}

func getToolDescriptions(overrides map[string]ToolOverride) map[string]*agent.ToolDescriptor {
	toolMap := map[string]*agent.ToolDescriptor{
		"login":                   &agent.LoginTool,
		"services":                &agent.ServicesTool,
		"deploy":                  &agent.DeployTool,
		"destroy":                 &agent.DestroyTool,
		"logs":                    &agent.LogsTool,
		"estimate":                &agent.EstimateTool,
		"set_config":              &agent.SetConfigTool,
		"remove_config":           &agent.RemoveConfigTool,
		"list_configs":            &agent.ListConfigsTool,
		"set_aws_provider":        &agent.SetAWSProvider,
		"set_gcp_provider":        &agent.SetGCPProvider,
		"set_playground_provider": &agent.SetPlaygroundProvider,
	}

	for toolKey, override := range overrides {
		if tool, exists := toolMap[toolKey]; exists {
			tool.Name = override.Name
			tool.Description = override.Description
		}
	}

	return toolMap
}

// TODO: Should match CollectTools.
func CollectMockTools(cluster string, authPort int, providerId *client.ProviderID) []ai.Tool {
	return CollectMockToolsWithConfig(cluster, authPort, providerId, nil)
}

func CollectMockToolsWithConfig(cluster string, authPort int, providerId *client.ProviderID, toolOverrides map[string]ToolOverride) []ai.Tool {
	// Apply overrides to agent tools if provided
	toolDefs := getToolDescriptions(toolOverrides)

	return []ai.Tool{
		ai.NewTool[agent.LoginParams, string](
			toolDefs["login"].Name,
			toolDefs["login"].Description,
			func(ctx *ai.ToolContext, params agent.LoginParams) (string, error) {
				return dummyToolHandler(ctx, toolDefs["login"].Name, params)
			},
		),
		ai.NewTool[agent.ServicesParams, string](
			toolDefs["services"].Name,
			toolDefs["services"].Description,
			func(ctx *ai.ToolContext, params agent.ServicesParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				var cli tools.CLIInterface = &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["services"].Name, loader, providerId, cluster, cli)
			},
		),

		ai.NewTool(toolDefs["deploy"].Name,
			toolDefs["deploy"].Description,
			func(ctx *ai.ToolContext, params agent.DeployParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["deploy"].Name, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(toolDefs["destroy"].Name,
			toolDefs["destroy"].Description,
			func(ctx *ai.ToolContext, params tools.DestroyParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["destroy"].Name, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(toolDefs["logs"].Name,
			toolDefs["logs"].Description,
			func(ctx *ai.ToolContext, params tools.LogsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["logs"].Name, loader, params, cluster, providerId, cli)
			},
		),
		ai.NewTool(toolDefs["estimate"].Name,
			toolDefs["estimate"].Description,
			func(ctx *ai.ToolContext, params tools.EstimateParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["estimate"].Name, loader, params, cluster, cli)
			},
		),
		ai.NewTool(toolDefs["set_config"].Name,
			toolDefs["set_config"].Description,
			func(ctx *ai.ToolContext, params tools.SetConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["set_config"].Name, loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool(toolDefs["remove_config"].Name,
			toolDefs["remove_config"].Description,
			func(ctx *ai.ToolContext, params tools.RemoveConfigParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["remove_config"].Name, loader, params, providerId, cluster, cli)
			},
		),
		ai.NewTool(toolDefs["list_configs"].Name,
			toolDefs["list_configs"].Description,
			func(ctx *ai.ToolContext, params tools.ListConfigsParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return dummyToolHandler(ctx, toolDefs["list_configs"].Name, loader, providerId, cluster, cli)
			},
		),
		ai.NewTool(toolDefs["set_aws_provider"].Name,
			toolDefs["set_aws_provider"].Description,
			func(ctx *ai.ToolContext, params tools.SetAWSProviderParams) (string, error) {
				return dummyToolHandler(ctx, toolDefs["set_aws_provider"].Name, params, providerId, cluster)
			},
		),
		ai.NewTool(toolDefs["set_gcp_provider"].Name,
			toolDefs["set_gcp_provider"].Description,
			func(ctx *ai.ToolContext, params tools.SetGCPProviderParams) (string, error) {
				return dummyToolHandler(ctx, toolDefs["set_gcp_provider"].Name, params, providerId, cluster)
			},
		),
		ai.NewTool(toolDefs["set_playground_provider"].Name,
			toolDefs["set_playground_provider"].Description,
			func(ctx *ai.ToolContext, params tools.SetPlaygroundProviderParams) (string, error) {
				return dummyToolHandler(ctx, toolDefs["set_playground_provider"].Name, params, providerId, cluster)
			},
		),
	}
}
