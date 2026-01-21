package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type StacksLoader interface {
	Load(ctx context.Context, name string) (*stacks.Parameters, error)
}

type StacksPutter interface {
	PutStack(ctx context.Context, req *defangv1.PutStackRequest) error
}

func SetDefaultStack(ctx context.Context, stacksPutter StacksPutter, stacksLoader StacksLoader, projectName, name string) error {
	stack, err := stacksLoader.Load(ctx, name)
	if err != nil {
		return err
	}

	stackfile, err := stacks.Marshal(stack)
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
			StackFile: []byte(stackfile),
		},
	})
	return err
}
