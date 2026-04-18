package cli

import (
	"context"
	"errors"
	"log/slog"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
)

func InstallCD(ctx context.Context, provider client.Provider, force bool) error {
	if dryrun.DoDryRun {
		return errors.New("dry run")
	}
	slog.Info("Installing the CD resources into the cluster")
	return provider.SetUpCD(ctx, force)
}
