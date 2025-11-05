package agent

import (
	"bytes"
	"context"
	"os"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
)

type Connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

type LoginParams struct{}
type ServicesParams struct {
	common.LoaderParams
}
type DeployParams struct {
	common.LoaderParams
}

// create a new Term that captures output to a buffer while also printing it to stdout
func captureTerm(f func() (string, error)) (string, error) {
	// replace the default term with a new term that writes to a buffer
	originalTerm := term.DefaultTerm
	outStream := bytes.NewBuffer(nil)
	errStream := bytes.NewBuffer(nil)
	newTerm := term.NewTerm(
		os.Stdin,
		outStream,
		errStream,
	)
	term.DefaultTerm = newTerm
	defer func() {
		term.DefaultTerm = originalTerm
	}()
	result, err := f()
	output := outStream.String() + errStream.String()
	return output + result, err
}

func CollectTools(cluster string, providerId *client.ProviderID) []ai.Tool {
	// loginHandler := MakeLoginToolHandler(cluster, authPort, &LoginCLIAdapter{DefaultToolCLI: &DefaultToolCLI{}})

	return []ai.Tool{
		ai.NewTool[LoginParams, string](
			"login",
			"Login into Defang",
			func(ctx *ai.ToolContext, _ LoginParams) (string, error) {
				return tools.HandleLoginTool(ctx.Context, cluster, &tools.DefaultToolCLI{})
			},
		),
		ai.NewTool[ServicesParams, string](
			"services",
			"List deployed services for the project in the current working directory",
			func(ctx *ai.ToolContext, params ServicesParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				var cli tools.CLIInterface = &tools.DefaultToolCLI{}
				return captureTerm(func() (string, error) {
					return tools.HandleServicesTool(ctx.Context, loader, providerId, cluster, cli)
				})
			},
		),
		ai.NewTool("deploy",
			"Deploy the application defined in the docker-compose files in the current working directory",
			func(ctx *ai.ToolContext, params DeployParams) (string, error) {
				loader, err := common.ConfigureAgentLoader(params.LoaderParams)
				if err != nil {
					return "Failed to configure loader", err
				}
				cli := &tools.DefaultToolCLI{}
				return captureTerm(func() (string, error) {
					return tools.HandleDeployTool(ctx.Context, loader, providerId, cluster, cli)
				})
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
				return captureTerm(func() (string, error) {
					return tools.HandleDestroyTool(ctx.Context, loader, providerId, cluster, cli)
				})
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
				return captureTerm(func() (string, error) {
					return tools.HandleLogsTool(ctx.Context, loader, params, cluster, providerId, cli)
				})
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
				return captureTerm(func() (string, error) {
					return tools.HandleEstimateTool(ctx.Context, loader, params, cluster, cli)
				})
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
				return captureTerm(func() (string, error) {
					return tools.HandleSetConfig(ctx.Context, loader, params, providerId, cluster, cli)
				})
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
				return captureTerm(func() (string, error) {
					return tools.HandleRemoveConfigTool(ctx.Context, loader, params, providerId, cluster, cli)
				})
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
				return captureTerm(func() (string, error) {
					return tools.HandleListConfigTool(ctx.Context, loader, providerId, cluster, cli)
				})
			},
		),
		ai.NewTool("set_aws_provider",
			"Set the AWS provider for the defang project",
			func(ctx *ai.ToolContext, params tools.SetAWSProviderParams) (string, error) {
				return tools.HandleSetAWSProvider(ctx.Context, params, providerId, cluster)
			},
		),
		ai.NewTool("set_gcp_provider",
			"Set the GCP provider for the defang project",
			func(ctx *ai.ToolContext, params tools.SetGCPProviderParams) (string, error) {
				return tools.HandleSetGCPProvider(ctx.Context, params, providerId, cluster)
			},
		),
		ai.NewTool("set_playground_provider",
			"Set the Playground provider for the defang project",
			func(ctx *ai.ToolContext, params tools.SetPlaygroundProviderParams) (string, error) {
				return tools.HandleSetPlaygroundProvider(providerId)
			},
		),
	}
}
