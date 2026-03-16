package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type StacksLoader interface {
	Load(ctx context.Context, name string) (*stacks.Parameters, error)
}

type StacksPutter interface {
	PutStack(ctx context.Context, req *defangv1.PutStackRequest) error
}

type StacksRemover interface {
	DeleteStack(ctx context.Context, req *defangv1.DeleteStackRequest) error
	ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error)
}

func SetDefaultStack(ctx context.Context, stacksPutter StacksPutter, stacksLoader StacksLoader, projectName, name string) error {
	stack, err := stacksLoader.Load(ctx, name)
	if err != nil {
		return err
	}

	stackFile, err := stacks.Marshal(stack)
	if err != nil {
		return err
	}

	err = stacksPutter.PutStack(ctx, &defangv1.PutStackRequest{
		Stack: &defangv1.Stack{
			Name:      stack.Name,
			Project:   projectName,
			Provider:  stack.Provider.Value(),
			Region:    stack.Region,
			Mode:      stack.Mode.Value(),
			IsDefault: true,
			StackFile: []byte(stackFile),
		},
	})
	return err
}

func RemoveStack(ctx context.Context, client StacksRemover, provider client.Provider, ec elicitations.Controller, projectName, name string, force bool) error {
	if !force {
		hasActiveDeployment, err := stackHasActiveDeployment(ctx, provider, projectName, name)
		if err != nil {
			return err
		}
		if hasActiveDeployment {
			confirmed, err := confirmRemoveStack(ctx, ec, name)
			if err != nil {
				return err
			}
			if !confirmed {
				return errors.New("stack deletion cancelled")
			}
		}
	}

	if err := client.DeleteStack(ctx, &defangv1.DeleteStackRequest{
		Project: projectName,
		Stack:   name,
	}); err != nil {
		return fmt.Errorf("failed to delete remote stack record: %w", err)
	}

	return stacks.RemoveInDirectory(".", name)
}

func stackHasActiveDeployment(ctx context.Context, provider client.Provider, projectName, name string) (bool, error) {
	list, err := provider.CdList(ctx, false)
	if err != nil {
		return false, fmt.Errorf("could not get list of deployed stacks: %w", err)
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

	for _, stack := range stacks {
		if stack.Project == projectName && stack.Stack == name {
			return true, nil
		}
	}

	return false, nil
}

func confirmRemoveStack(ctx context.Context, ec elicitations.Controller, name string) (bool, error) {
	if !ec.IsSupported() {
		return false, fmt.Errorf("stack %q has an active deployment; re-run in interactive mode to confirm deletion", name)
	}
	prompt := fmt.Sprintf(
		`Stack %q has an active deployment. In order to avoid orphaned resources and unexpected costs, we recommend running 'defang down -s %s' to spin down the stack before deletion.

If you choose to proceed without spinning the stack down, be aware that Defang will lose the ability to manage or track these resources.

Are you sure you want to delete it?`,
		name,
		name,
	)
	answer, err := ec.RequestEnum(ctx,
		prompt,
		"confirm",
		[]string{"yes", "no"},
	)
	if err != nil {
		return false, err
	}
	return answer == "yes", nil
}
