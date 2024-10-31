package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func LoadProjectName(ctx context.Context, loader compose.Loader, provider client.Provider) (string, error) {
	project, err := loader.LoadProject(ctx)
	if err == nil {
		return project.Name, nil
	}

	term.Debug("Failed to load local project:", err)
	term.Debug("Trying to get the remote project name from the provider")
	projectName, err := provider.RemoteProjectName(ctx)
	if err != nil {
		return "", err
	}
	return projectName, nil
}
