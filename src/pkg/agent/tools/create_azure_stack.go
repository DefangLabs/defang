package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type CreateAzureStackParams struct {
	common.LoaderParams
	Name                string `json:"stack" jsonschema:"required,description=The name of the stack to use for all tool calls."`
	Location            string `json:"location" jsonschema:"required,description=The Azure location to create the stack in."`
	AzureSubscriptionID string `json:"azure_subscription_id" jsonschema:"required,description=The Azure subscription ID to use when creating the stack."`
	Mode                string `json:"mode" jsonschema:"enum=affordable,enum=balanced,enum=high_availability,description=The deployment mode for the stack."`
}

func HandleCreateAzureStackTool(ctx context.Context, params CreateAzureStackParams, sc StackConfig) (string, error) {
	newStack := stacks.Parameters{
		Name:     params.Name,
		Region:   params.Location,
		Provider: client.ProviderAzure,
		Mode:     modes.Parse(params.Mode),
		Variables: map[string]string{
			"AZURE_SUBSCRIPTION_ID": params.AzureSubscriptionID,
		},
	}
	return createAndLoadStack(newStack, params.WorkingDirectory, sc)
}
