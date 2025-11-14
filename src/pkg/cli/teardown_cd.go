package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func TearDownCD(ctx context.Context, provider client.Provider, force bool) error {
	if dryrun.DoDryRun {
		return errors.New("dry run")
	}
	if !force {
		if list, err := provider.BootstrapList(ctx); err != nil {
			return fmt.Errorf("could not get list of services; use --force to tear down anyway: %w", err)
		} else if len(list) > 0 {
			return errors.New("there are still deployed services; use --force to tear down anyway")
		}
	}
	term.Warn("Deleting the CD cluster; this does not delete services or configs!")
	return provider.TearDownCD(ctx)
}
