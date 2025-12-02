package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func InstallCD(ctx context.Context, provider client.Provider) error {
	if dryrun.DoDryRun {
		return errors.New("dry run")
	}
	term.Info("Installing the CD resources into the cluster")
	return provider.SetUpCD(ctx)
}
