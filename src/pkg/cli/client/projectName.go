package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func LoadProjectNameWithFallback(ctx context.Context, loader Loader, provider Provider) (string, error) {
	if loader != nil {
		projectName, err := loader.LoadProjectName(ctx)
		if err == nil {
			return projectName, nil
		}
		term.Debug("Failed to load local project:", err)
	}
	term.Debug("Trying to get the remote project name from the provider")
	projectName, err := provider.RemoteProjectName(ctx)
	if err != nil {
		return "", err
	}
	return projectName, nil
}
