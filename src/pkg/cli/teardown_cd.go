package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
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

	var stacks []state.StackInfo
	for stackInfo := range list {
		stacks = append(stacks, *stackInfo)
	}

	if len(stacks) > 0 {
		term.Info("There following stacks are currently deployed:")
		for _, stack := range stacks {
			term.Infof(" - (%s) %s/%s [%s]\n", stack.Workspace, stack.Project, stack.Name, stack.Region)
		}
		if !force {
			return ErrExistingStacks
		}
	}

	return provider.TearDownCD(ctx)
}
