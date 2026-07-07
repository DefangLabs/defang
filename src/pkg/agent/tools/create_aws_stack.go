package tools

import (
	"context"

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
	Recipe      string `json:"recipe" jsonschema:"enum=affordable,enum=balanced,enum=high_availability,description=The deployment mode/recipe for the stack."`
}

func HandleCreateAWSStackTool(ctx context.Context, params CreateAWSStackParams, sc StackConfig) (string, error) {
	newStack := stacks.Parameters{
		Name:     params.Name,
		Region:   params.Region,
		Provider: client.ProviderAWS,
		Recipe:   modes.ParseRecipe(params.Recipe),
		Variables: map[string]string{
			"AWS_PROFILE": params.AWS_Profile,
		},
	}
	return createAndLoadStack(newStack, params.WorkingDirectory, sc)
}
