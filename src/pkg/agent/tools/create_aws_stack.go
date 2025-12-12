package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type CreateAWSStackParams struct {
	common.LoaderParams
	Name        string `json:"stack" jsonschema:"required,description=The name of the stack to use for all tool calls."`
	Region      string `json:"region" jsonschema:"required,description=The AWS region to create the stack in."`
	AWS_Profile string `json:"aws_profile" jsonschema:"required,description=The AWS profile to use when creating the stack."`
	Mode        string `json:"mode" jsonschema:"enum=affordable,enum=balaned,enum=high_availability,description=The deployment mode for the stack."`
}

func HandleCreateAWSStackTool(ctx context.Context, params CreateAWSStackParams, sc StackConfig) (string, error) {
	var mode modes.Mode
	var err error
	if params.Mode == "" {
		mode = modes.ModeAffordable
	} else {
		mode, err = modes.Parse(params.Mode)
		if err != nil {
			return "Invalid mode provided", err
		}
	}

	newStack := stacks.StackParameters{
		Name:       params.Name,
		AWSProfile: params.AWS_Profile,
		Provider:   cliClient.ProviderAWS,
		Region:     params.Region,
		Mode:       mode,
	}

	_, err = stacks.Create(newStack)
	if err != nil {
		return "Failed to create stack", err
	}

	return fmt.Sprintf("Successfully created stack %q. Use the 'select_stack' tool to activate it for use.", params.Name), nil
}
