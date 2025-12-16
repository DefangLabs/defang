package stacks

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/elicitations"
)

const CreateNewStack = "Create new stack"

type stackSelector struct {
	ec elicitations.Controller
	sm Manager
}

func NewSelector(ec elicitations.Controller, sm Manager) *stackSelector {
	return &stackSelector{
		ec: ec,
		sm: sm,
	}
}

func (ss *stackSelector) SelectStack(ctx context.Context) (*StackParameters, error) {
	if !ss.ec.IsSupported() {
		return nil, errors.New("your mcp client does not support elicitations, use the 'select_stack' tool to choose a stack")
	}
	selectedStackName, err := ss.elicitStackSelection(ctx, ss.ec)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	if selectedStackName == CreateNewStack {
		wizard := NewWizard(ss.ec)
		params, err := wizard.CollectParameters(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect stack parameters: %w", err)
		}
		_, err = ss.sm.Create(*params)
		if err != nil {
			return nil, fmt.Errorf("failed to create stack: %w", err)
		}

		selectedStackName = params.Name
	}

	return ss.sm.Load(selectedStackName)
}

func (ss *stackSelector) elicitStackSelection(ctx context.Context, ec elicitations.Controller) (string, error) {
	stackList, err := ss.sm.List(ctx)
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
