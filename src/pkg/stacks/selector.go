package stacks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
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

	printStacksInfoMessage(stackNames)
	selectedStackName, err := ec.RequestEnum(ctx, "Select a stack", "stack", stackNames)
	if err != nil {
		return "", fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	return selectedStackName, nil
}

func printStacksInfoMessage(stacks []string) {
	// If there is a stack named "beta", print an informational message about it
	betaExists := slices.Contains(stacks, "beta")
	if betaExists {
		infoLine := "This project was deployed with an implicit Stack called 'beta' before Stacks were introduced."
		if len(stacks) == 1 {
			infoLine += "\n   To update your existing deployment, select the 'beta' Stack.\n" +
				"Creating a new Stack will result in a separate deployment instance."
		}
		infoLine += "\n   To learn more about Stacks, visit: https://docs.defang.io/docs/concepts/stacks"
		term.Info(infoLine + "\n")
	}
	executable, _ := os.Executable()
	term.Infof("To skip this prompt, run %s up --stack=%s", filepath.Base(executable), "<stack_name>")
}
