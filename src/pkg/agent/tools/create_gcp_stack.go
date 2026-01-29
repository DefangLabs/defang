package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

type CreateGCPStackParams struct {
	common.LoaderParams
	Name         string `json:"stack" jsonschema:"required,description=The name of the stack to use for all tool calls."`
	Region       string `json:"region" jsonschema:"required,description=The GCP region to create the stack in."`
	GCPProjectID string `json:"gcp_project_id" jsonschema:"required,description=The GCP project ID to use when creating the stack."`
	Mode         string `json:"mode" jsonschema:"enum=affordable,enum=balanced,enum=high_availability,description=The deployment mode for the stack."`
}

func HandleCreateGCPStackTool(ctx context.Context, params CreateGCPStackParams, sc StackConfig) (string, error) {
	return createStack(createStackParams{
		WorkingDirectory: params.WorkingDirectory,
		Name:             params.Name,
		Region:           params.Region,
		Provider:         client.ProviderGCP,
		Mode:             params.Mode,
		Variables:        map[string]string{"GCP_PROJECT_ID": params.GCPProjectID},
	}, sc)
}
