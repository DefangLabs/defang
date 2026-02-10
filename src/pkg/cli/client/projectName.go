package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/term"
)

// Deprecated: should use stacks instead of ProjectName fallback.
func LoadProjectNameWithFallback(ctx context.Context, loader Loader, provider Provider) (string, error) {
	projectName, _, err := loader.LoadProjectName(ctx)
	if err != nil {
		term.Debug("Failed to load local project:", err)
		return "", err
	}
	return projectName, nil
}
