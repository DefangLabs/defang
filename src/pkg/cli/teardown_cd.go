package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
)

var ErrExistingStacks = errors.New("there are still deployed stacks")

func TearDownCD(ctx context.Context, provider client.Provider, force bool) error {
	if dryrun.DoDryRun {
		return errors.New("dry run")
	}
	list, err := provider.CdList(ctx, false)
	if err != nil {
		return fmt.Errorf("could not get list of deployed stacks: %w", err)
	}
	stacks := slices.SortedFunc(list, func(a, b state.Info) int {
		if a.Workspace != b.Workspace {
			return cmp.Compare(a.Workspace, b.Workspace)
		}
		if a.Project != b.Project {
			return cmp.Compare(a.Project, b.Project)
		}
		return cmp.Compare(a.Stack, b.Stack)
	})

	if len(stacks) > 0 {
		slog.InfoContext(ctx, "Some stacks are currently deployed. Run the following commands to tear them down:")
		for _, stack := range stacks {
			slog.InfoContext(ctx, fmt.Sprintf("  `defang down --workspace %s --project-name %s --stack %s`", stack.Workspace, stack.Project, stack.Stack))
		}
		if !force {
			return ErrExistingStacks
		}
	}

	return provider.TearDownCD(ctx)
}
