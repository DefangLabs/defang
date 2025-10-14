package agent

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
)

type Connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

type LoginParams struct{}

func CollectTools(cluster string) []ai.Tool {
	// loginHandler := MakeLoginToolHandler(cluster, authPort, &LoginCLIAdapter{DefaultToolCLI: &DefaultToolCLI{}})

	return []ai.Tool{
		ai.NewTool[LoginParams, string](
			"login",
			"Login into Defang",
			func(ctx *ai.ToolContext, params LoginParams) (string, error) {
				// return loginHandler(ctx.Context, params)
				return "Login tool is not yet implemented", nil
			},
		),
	}
}
