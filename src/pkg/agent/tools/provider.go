package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const CreateNewStack = "Create new stack"

type ProviderCreator interface {
	NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient, stack string) client.Provider
}

type StacksManager interface {
	List(ctx context.Context) ([]stacks.ListItem, error)
	Load(ctx context.Context, name string) (*stacks.Parameters, error)
	LoadLocal(name string) (*stacks.Parameters, error)
	LoadRemote(ctx context.Context, name string) (*stacks.Parameters, error)
	LoadStackEnv(params stacks.Parameters, overload bool) error
	Create(params stacks.Parameters) (string, error)
	TargetDirectory() string
}

type providerPreparer struct {
	pc ProviderCreator
	ec elicitations.Controller
	fc client.FabricClient
	sm StacksManager
}

func NewProviderPreparer(pc ProviderCreator, ec elicitations.Controller, fc client.FabricClient, sm StacksManager) *providerPreparer {
	return &providerPreparer{
		pc: pc,
		ec: ec,
		fc: fc,
		sm: sm,
	}
}

func (pp *providerPreparer) SetupProvider(ctx context.Context, stack *stacks.Parameters) (*client.ProviderID, client.Provider, error) {
	if stack.Name == "" && pp.ec.IsSupported() {
		selector := stacks.NewSelector(pp.ec, pp.sm)
		newStack, err := selector.SelectStack(ctx, stacks.SelectStackOptions{
			AllowCreate: true,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup stack: %w", err)
		}
		*stack = *newStack
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider := pp.pc.NewProvider(ctx, stack.Provider, pp.fc, stack.Name)
	providerID := stack.Provider
	return &providerID, provider, nil
}
