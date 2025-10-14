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

type LoginParams struct{}
type ServicesParams struct {
	common.LoaderParams
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
				return tools.HandleServicesTool(ctx.Context, loader, providerId, cluster, cli)
			},
		),
	}
}
