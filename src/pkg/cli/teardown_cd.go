package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
)

var ErrExistingProjects = errors.New("there are still deployed projects")

func TearDownCD(ctx context.Context, provider client.Provider, force bool) error {
	if dryrun.DoDryRun {
		return errors.New("dry run")
	}
	if !force {
		list, err := provider.CdList(ctx, false)
		if err != nil {
			return fmt.Errorf("could not get list of projects: %w", err)
		}

		found := false
		for project := range list {
			if !found {
				term.Info("There are still deployed projects:")
			}
			fmt.Println(project)
			found = true
		}
		if found {
			return ErrExistingProjects
		}
	}
	term.Warn("Deleting the CD cluster; this does not delete projects or configs!")
	return provider.TearDownCD(ctx)
}
