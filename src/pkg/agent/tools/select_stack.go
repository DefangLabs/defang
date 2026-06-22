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
	// TODO: this only loads local stack files, not remote stacks (e.g. created in Portal)
	stack, err := stacks.ReadInDirectory(params.WorkingDirectory, params.Stack)
	if err != nil {
		return "", fmt.Errorf("Unable to load stack %q, please use the tools create_aws_stack or create_gcp_stack or create_azure_stack to create a stack: %w", params.Stack, err)
	}

	err = loadStack(*stack, sc)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Stack %q selected.", sc.Stack.Name), nil
}

func loadStack(stack stacks.Parameters, sc StackConfig) error {
	err := stacks.LoadStackEnv(stack, true)
	if err != nil {
		return fmt.Errorf("Unable to load stack %q: %w", stack.Name, err)
	}

	*sc.Stack = stack
	return nil
}

func createAndLoadStack(newStack stacks.Parameters, dir string, sc StackConfig) (string, error) {
	_, err := stacks.CreateInDirectory(dir, newStack)
	if err != nil {
		return "Failed to create stack", err
	}

	err = loadStack(newStack, sc)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully created stack %q and loaded its environment.", newStack.Name), nil
}
