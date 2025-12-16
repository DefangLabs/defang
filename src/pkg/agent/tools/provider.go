package tools

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const CreateNewStack = "Create new stack"

type ProviderCreator interface {
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient, stack string) cliClient.Provider
}

type providerPreparer struct {
	pc ProviderCreator
	ec elicitations.Controller
	fc cliClient.FabricClient
	sm stacks.Manager
}

func NewProviderPreparer(pc ProviderCreator, ec elicitations.Controller, fc cliClient.FabricClient, sm stacks.Manager) *providerPreparer {
	return &providerPreparer{
		pc: pc,
		ec: ec,
		fc: fc,
		sm: sm,
	}
}

func (pp *providerPreparer) SetupProvider(ctx context.Context, stack *stacks.StackParameters) (*cliClient.ProviderID, cliClient.Provider, error) {
	var providerID cliClient.ProviderID
	var err error
	if stack.Name == "" {
		selector := stacks.NewSelector(pp.ec, pp.sm)
		newStack, err := selector.SelectStack(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup stack: %w", err)
		}
		*stack = *newStack
	}

	err = providerID.Set(stack.Provider.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to set provider ID: %w", err)
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider := pp.pc.NewProvider(ctx, providerID, pp.fc, stack.Name)
	return &providerID, provider, nil
}
