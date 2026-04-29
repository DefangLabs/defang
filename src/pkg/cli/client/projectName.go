package client

import (
	"context"
	"fmt"
	"log/slog"
)

// Deprecated: should use stacks instead of ProjectName fallback.
func LoadProjectNameWithFallback(ctx context.Context, loader Loader, provider Provider) (string, error) {
	var loadErr error
	if loader != nil {
		projectName, _, err := loader.LoadProjectName(ctx)
		if err == nil {
			return projectName, nil
		}
		slog.Debug(fmt.Sprint("Failed to load local project:", err))
		loadErr = err
	}
	slog.Debug("Trying to get the remote project name from the provider")
	projectName, err := provider.RemoteProjectName(ctx)
	if err != nil {
		return "", fmt.Errorf("%w and %w", loadErr, err)
	}
	return projectName, nil
}
