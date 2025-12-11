package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type AccessDataParams struct {
	common.LoaderParams
}

func HandleAccessDataTool(ctx context.Context, loader cliClient.ProjectLoader, params AccessDataParams, sc *StackConfig) (string, error) {
	current := stacks.StackListItem{
		Name:       sc.Stack.Name,
		AWSProfile: sc.Stack.AWSProfile,
		Provider:   sc.Stack.Provider.String(),
		Region:     sc.Stack.Region,
		Mode:       sc.Stack.Mode.String(),
	}

	return fmt.Sprintf("This is the stack in memory: %v", current), nil
}
