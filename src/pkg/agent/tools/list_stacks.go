package tools

import (
	"context"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type ListStacksParams struct {
	common.LoaderParams
}

func HandleListStacksTool(ctx context.Context, params ListStacksParams) (string, error) {
	stacksList, err := stacks.ListInDirectory(params.WorkingDirectory)
	if err != nil {
		return "", err
	}

	if len(stacksList) == 0 {
		return "No stacks found.", nil
	}

	// Extract stack names for display
	stackNames := make([]string, len(stacksList))
	for i, stack := range stacksList {
		stackNames[i] = stack.Name
	}

	return "Available stacks:\n- " + strings.Join(stackNames, ", "), nil
}
