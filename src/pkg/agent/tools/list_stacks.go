package tools

import (
	"context"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/stacks"
)

func HandleListStacksTool(ctx context.Context) (string, error) {
	stacksList, err := stacks.List()
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
