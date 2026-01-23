package stacks

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const CreateNewStack = "Create new stack"

type Manager interface {
	List(ctx context.Context) ([]ListItem, error)
	Create(params Parameters) (string, error)
}

type stackSelector struct {
	ec     elicitations.Controller
	sm     Manager
	wizard *Wizard
}

func NewSelector(ec elicitations.Controller, sm Manager) *stackSelector {
	return &stackSelector{
		ec:     ec,
		sm:     sm,
		wizard: NewWizard(ec),
	}
}

type SelectStackOptions struct {
	AllowStackCreation bool
}

func (ss *stackSelector) SelectStack(ctx context.Context, opts SelectStackOptions) (*Parameters, error) {
	if !ss.ec.IsSupported() {
		return nil, errors.New("your MCP client does not support elicitations, use the 'select_stack' tool to choose a stack")
	}
	stackList, err := ss.sm.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list stacks: %w", err)
	}

	var selectedName string
	if len(stackList) == 0 {
		if opts.AllowStackCreation {
			return ss.createStack(ctx)
		} else {
			return nil, errors.New("no stacks available to select")
		}
	}

	stackLabels := make([]string, 0, len(stackList)+1)
	stackNames := make([]string, 0, len(stackList))
	labelMap := make(map[string]string)
	for _, s := range stackList {
		var label string
		if s.DeployedAt.IsZero() {
			label = s.Name
		} else {
			label = fmt.Sprintf("%s (deployed %s)", s.Name, s.DeployedAt.Format("Jan 2 2006"))
		}
		stackLabels = append(stackLabels, label)
		stackNames = append(stackNames, s.Name)
		labelMap[label] = s.Name
	}
	if opts.AllowStackCreation {
		stackLabels = append(stackLabels, CreateNewStack)
	}

	printStacksInfoMessage(stackNames)
	selectedLabel, err := ss.ec.RequestEnum(ctx, "Select a stack", "stack", stackLabels)
	if err != nil {
		return nil, fmt.Errorf("failed to elicit stack choice: %w", err)
	}

	// If "Create new stack" was selected, return as-is
	if selectedLabel == CreateNewStack {
		return ss.createStack(ctx)
	}

	// Otherwise, map back to the actual stack name
	selectedName, exists := labelMap[selectedLabel]
	if !exists {
		return nil, fmt.Errorf("invalid stack selection: %s", selectedLabel)
	}

	// find the stack with the selected name in the list of stacks
	for _, stack := range stackList {
		if stack.Name == selectedName {
			return &stack.Parameters, nil
		}
	}

	return nil, fmt.Errorf("selected stack %q not found", selectedName)
}

func (ss *stackSelector) createStack(ctx context.Context) (*Parameters, error) {
	params, err := ss.wizard.CollectParameters(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect stack parameters: %w", err)
	}
	_, err = ss.sm.Create(*params)
	if err != nil {
		return nil, fmt.Errorf("failed to create stack: %w", err)
	}

	return params, nil
}

func printStacksInfoMessage(stacks []string) {
	// If there is a stack named "beta", print an informational message about it
	betaExists := slices.Contains(stacks, DefaultBeta)
	if betaExists {
		infoLine := "This project was deployed with an implicit Stack called 'beta' before Stacks were introduced."
		if len(stacks) == 1 {
			infoLine += "\n   To update your existing deployment, select the 'beta' Stack.\n" +
				"Creating a new Stack will result in a separate deployment instance."
		}
		infoLine += "\n   To learn more about Stacks, visit: https://docs.defang.io/docs/concepts/stacks"
		term.Println(infoLine)
	}
	term.Printf("To skip this prompt, run this command with --stack=%s\n", "<stack_name>")
}
