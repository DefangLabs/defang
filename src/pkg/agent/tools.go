package agent

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
)

type Connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

func CollectTools(cluster string, authPort int) []ai.Tool {
	loginHandler := MakeLoginToolHandler(cluster, authPort, &LoginCLIAdapter{DefaultToolCLI: &DefaultToolCLI{}})

	return []ai.Tool{
		ai.NewTool[LoginParams, string](
			"login",
			"Login into Defang",
			func(ctx *ai.ToolContext, _ LoginParams) (string, error) {
				return tools.HandleLoginTool(ctx.Context, cluster, authPort, &tools.LoginCLIAdapter{DefaultToolCLI: &tools.DefaultToolCLI{}})
			},
		),
	}
}
