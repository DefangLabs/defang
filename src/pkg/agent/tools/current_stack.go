package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/stacks"
)

func HandleCurrentStackTool(ctx context.Context, sc StackConfig) (string, error) {
	if sc.Stack.Name == "" {
		return "No stack is currently selected.", nil
	}

	details, err := stacks.Marshal(sc.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to marshal stack details: %w", err)
	}
	return fmt.Sprintf("This currently selected stack is %q: %v", sc.Stack.Name, details), nil
}
