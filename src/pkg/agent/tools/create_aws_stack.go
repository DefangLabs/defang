package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type CreateAWSStackParams struct {
	common.LoaderParams
	Name        string `json:"stack" jsonschema:"required,description=The name of the stack to use for all tool calls."`
	Region      string `json:"region" jsonschema:"required,description=The AWS region to create the stack in."`
	AWS_Profile string `json:"aws_profile" jsonschema:"required,description=The AWS profile to use when creating the stack."`
	Mode        string `json:"mode" jsonschema:"enum=affordable,enum=balanced,enum=high_availability,description=The deployment mode for the stack."`
}

func HandleCreateAWSStackTool(ctx context.Context, params CreateAWSStackParams, sc StackConfig) (string, error) {
	if params.Mode == "" {
		params.Mode = modes.ModeAffordable.String()
	}
	mode, err := modes.Parse(params.Mode)
	if err != nil {
		return "Invalid mode provided", err
	}

	newStack := stacks.Parameters{
		Name:     params.Name,
		Region:   params.Region,
		Provider: client.ProviderAWS,
		Mode:     mode,
		Variables: map[string]string{
			"AWS_PROFILE": params.AWS_Profile,
		},
	}
	return createAndLoadStack(newStack, params.WorkingDirectory, sc)
}

func createAndLoadStack(newStack stacks.Parameters, dir string, sc StackConfig) (string, error) {
	_, err := stacks.CreateInDirectory(dir, newStack)
	if err != nil {
		return "Failed to create stack", err
	}

	err = stacks.LoadStackEnv(newStack, true)
	if err != nil {
		return "", fmt.Errorf("Unable to load stack %q: %w", newStack.Name, err)
	}
	if sc.Stack == nil {
		return "", errors.New("stack config not initialized")
	}

	*sc.Stack = newStack

	return fmt.Sprintf("Successfully created stack %q and loaded its environment.", newStack.Name), nil
}
