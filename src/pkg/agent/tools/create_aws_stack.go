package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

type CreateAWSStackParams struct {
	common.LoaderParams
	Name        string `json:"stack" jsonschema:"required,description=The name of the stack to use for all tool calls."`
	Region      string `json:"region" jsonschema:"required,description=The AWS region to create the stack in."`
	AWS_Profile string `json:"aws_profile" jsonschema:"required,description=The AWS profile to use when creating the stack."`
	Mode        string `json:"mode" jsonschema:"enum=affordable,enum=balanced,enum=high_availability,description=The deployment mode for the stack."`
}

func HandleCreateAWSStackTool(ctx context.Context, params CreateAWSStackParams, sc StackConfig) (string, error) {
	return createStack(createStackParams{
		WorkingDirectory: params.WorkingDirectory,
		Name:             params.Name,
		Region:           params.Region,
		Provider:         client.ProviderAWS,
		Mode:             params.Mode,
		Variables:        map[string]string{"AWS_PROFILE": params.AWS_Profile},
	}, sc)
}
