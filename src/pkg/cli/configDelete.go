package cli

import (
	"context"
	"log/slog"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigDelete(ctx context.Context, projectName string, provider client.Provider, names ...string) error {
	slog.Debug("Deleting config in project", "names", names, "project", projectName)

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	return provider.DeleteConfig(ctx, &defangv1.Secrets{Names: names, Project: projectName})
}
