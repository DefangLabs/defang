package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type SelectStackParams struct {
	common.LoaderParams
	Stack string `json:"stack" jsonschema:"required,description=The name of the stack to use."`
}

func HandleSelectStackTool(ctx context.Context, params SelectStackParams, sc StackConfig) (string, error) {
	stack, err := stacks.ReadInDirectory(params.WorkingDirectory, params.Stack)
	if err != nil {
		return "", fmt.Errorf("Unable to load stack %q, please use the tools create_aws_stack to create a stack for AWS deployment or create_gcp_stack to create a stack for GCP deployment: %w", params.Stack, err)
	}

	stacks.LoadParameters(*stack, true)
	if err != nil {
		return "", fmt.Errorf("Unable to load stack %q: %w", params.Stack, err)
	}

	*sc.Stack = *stack

	return fmt.Sprintf("Stack %q selected.", sc.Stack.Name), nil
}
