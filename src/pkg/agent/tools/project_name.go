package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

type ProjectNameParams struct {
	common.LoaderParams
}

// HandleProjectNameTool handles the project name tool logic
func HandleProjectNameTool(ctx context.Context, loader client.Loader) (string, error) {
	pn, _, err := loader.LoadProjectName(ctx)
	if err != nil {
		return "", err
	}
	return pn, nil
}
