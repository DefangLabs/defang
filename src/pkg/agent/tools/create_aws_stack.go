package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type CreateAWSStackParams struct {
	common.LoaderParams
	Name        *string `json:"stack" jsonschema:"description=The name of the stack to use for all tool calls."`
	Region      *string `json:"region" jsonschema:"description=The AWS region to create the stack in."`
	AWS_Profile *string `json:"aws_profile" jsonschema:"description=The AWS profile to use when creating the stack."`
	Mode        *string `json:"mode" jsonschema:"description=The deployment mode for the stack."`
}

func HandleCreateAWSStackTool(ctx context.Context, loader cliClient.ProjectLoader, cli CLIInterface, params CreateAWSStackParams, sc *StackConfig) (string, error) {
	mode, err := modes.Parse(*params.Mode)
	if err != nil {
		return "Invalid mode provided", err
	}

	newStack := stacks.StackParameters{
		Name:       *params.Name,
		AWSProfile: *params.AWS_Profile,
		Provider:   cliClient.ProviderAWS,
		Region:     *params.Region,
		Mode:       mode,
	}

	fileName, err := stacks.Create(newStack)
	if err != nil {
		return "Failed to create AWS stack", err
	}

	return "AWS stack created successfully: " + fileName, nil
}
