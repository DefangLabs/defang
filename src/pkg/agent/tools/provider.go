package tools

import (
	"context"
	"errors"
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
		newStack, err := pp.setupStack(ctx)
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

func (pp *providerPreparer) selectStack(ctx context.Context, ec elicitations.Controller) (string, error) {
	stackList, err := pp.sm.List(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list stacks: %w", err)
	}

	if len(stackList) == 0 {
		return CreateNewStack, nil
	}

	stackNames := make([]string, 0, len(stackList)+1)
	for _, s := range stackList {
		stackNames = append(stackNames, s.Name)
	}
	stackNames = append(stackNames, CreateNewStack)

	selectedStackName, err := ec.RequestEnum(ctx, "Select a stack", "stack", stackNames)
	if err != nil {
		return "", fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	return selectedStackName, nil
}

func (pp *providerPreparer) setupStack(ctx context.Context) (*stacks.StackParameters, error) {
	if !pp.ec.IsSupported() {
		return nil, errors.New("your mcp client does not support elicitations, use the 'select_stack' tool to choose a stack")
	}
	selectedStackName, err := pp.selectStack(ctx, pp.ec)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	if selectedStackName == CreateNewStack {
		wizard := stacks.NewWizard(pp.ec)
		params, err := wizard.CollectParameters(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect stack parameters: %w", err)
		}
		_, err = pp.sm.Create(*params)
		if err != nil {
			return nil, fmt.Errorf("failed to create stack: %w", err)
		}

		selectedStackName = params.Name
	}

	return pp.sm.Load(selectedStackName)
}
