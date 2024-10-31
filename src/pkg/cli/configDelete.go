package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigDelete(ctx context.Context, loader client.Loader, provider client.Provider, names ...string) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Deleting config %v in project %q", names, projectName)

	if DoDryRun {
		return ErrDryRun
	}

	return provider.DeleteConfig(ctx, &defangv1.Secrets{Names: names, Project: projectName})
}
