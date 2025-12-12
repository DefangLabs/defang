package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type SelectStackParams struct {
	common.LoaderParams
	Stack *string `json:"stack" jsonschema:"description=The name of the stack to use for all tool calls."`
}

func HandleSelectStackTool(ctx context.Context, params SelectStackParams, sc StackConfig) (string, error) {
	// User shouldn't need to be require to select a stack
	if params.Stack != nil {
		stack, err := stacks.Read(*params.Stack)
		if err != nil {
			return "", fmt.Errorf("Unable to load stack %q, please use the tools awsStackcreate to create a stack for AWS deployment or gcpStackcreate to create a stack for GCP deployment: %w", *params.Stack, err)
		}

		sc.Stack = stack
	}

	return fmt.Sprintf("Stack %q selected for tool calls.", sc.Stack), nil
}
