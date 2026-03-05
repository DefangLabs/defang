package cli

import (
	"context"
	"errors"
	"fmt"

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

func RemoveStack(ctx context.Context, client StacksRemover, ec elicitations.Controller, projectName, name string) error {
	resp, err := client.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: projectName,
		Stack:   name,
		Limit:   1,
	})
	if err != nil {
		return fmt.Errorf("failed to list deployments for stack %q: %w", name, err)
	}

	if len(resp.Deployments) > 0 && resp.Deployments[0].Action == defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP {
		if !ec.IsSupported() {
			return fmt.Errorf("stack %q has an active deployment; re-run in interactive mode to confirm deletion", name)
		}
		answer, err := ec.RequestEnum(ctx,
			fmt.Sprintf("Stack %q may still have active resources. Deleting this stack will not modify or remove any active resources, it will only remove Defang's ability to manage them. Are you sure you want to delete it?", name),
			"confirm",
			[]string{"yes", "no"},
		)
		if err != nil {
			return err
		}
		if answer != "yes" {
			return errors.New("stack deletion cancelled")
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
