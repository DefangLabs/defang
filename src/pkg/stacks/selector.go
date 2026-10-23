package stacks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const CreateNewStack = "Create new stack"

type Manager interface {
	List(ctx context.Context) ([]ListItem, error)
	Load(ctx context.Context, name string) (*Parameters, error)
	Create(params Parameters) (string, error)
}

type stackSelector struct {
	ec     elicitations.Controller
	sm     Manager
	wizard *Wizard
}

func NewSelector(ec elicitations.Controller, sm Manager, recipeLister RecipeLister) *stackSelector {
	return &stackSelector{
		ec:     ec,
		sm:     sm,
		wizard: NewWizard(ec, recipeLister),
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
			return nil, errors.New("no stacks available to select in this workspace")
		}
	}
	labelMap := makeStackSelectorLabels(stackList)
	stackLabels := make([]string, 0, len(stackList)+1)
	stackNames := make([]string, 0, len(stackList))
	for _, stack := range stackList {
		for label, name := range labelMap {
			if name == stack.Name {
				stackLabels = append(stackLabels, label)
				stackNames = append(stackNames, name)
				break
			}
		}
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
	stack, err := ss.sm.Load(ctx, selectedName)
	if err != nil {
		return nil, fmt.Errorf("failed to load stack %q: %w", selectedName, err)
	}
	return stack, nil
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
			infoLine += "\n - To update your existing deployment, select the 'beta' Stack.\n" +
				" - Creating a new Stack will result in a separate deployment instance."
		}
		infoLine += "\n - To learn more about Stacks, visit: https://s.defang.io/stacks"
		term.Println(infoLine)
	}
	term.Printf("To skip this prompt, run this command with --stack=%s\n", "<stack_name>")
}

func makeStackSelectorLabels(stacks []ListItem) map[string]string {
	partsList := stackLabelParts(stacks)
	partsList = reduceStackLabelParts(partsList)

	labelMap := make(map[string]string)
	for i, parts := range partsList {
		label := formatStackLabelParts(parts)
		labelMap[label] = stacks[i].Name
	}
	return labelMap
}

func stackLabelParts(stacks []ListItem) [][]string {
	partsList := make([][]string, len(stacks))
	for i, s := range stacks {
		var deployedAt string
		if !s.DeployedAt.IsZero() {
			deployedAt = "last deployed " + s.DeployedAt.Format(time.UnixDate)
		}
		partsList[i] = []string{
			s.Name,
			s.Provider.String(),
			s.Account,
			s.Region,
			deployedAt,
		}
	}
	return partsList
}

func reduceStackLabelParts(partsList [][]string) [][]string {
	if len(partsList) <= 1 {
		return partsList
	}
	// iterate over the partsList,
	// if all stacks have the same value for a given part index, remove that part from all labels
	for i := 0; i < len(partsList[0]); i++ {
		same := true
		value := partsList[0][i]
		for _, part := range partsList {
			if part[i] != value {
				same = false
				break
			}
		}
		if same {
			for j := 0; j < len(partsList); j++ {
				partsList[j] = append(partsList[j][:i], partsList[j][i+1:]...)
			}
			i-- // adjust index since we removed a part
		}
	}

	return partsList
}

func formatStackLabelParts(parts []string) string {
	// remove any empty parts
	nonEmptyParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			nonEmptyParts = append(nonEmptyParts, part)
		}
	}
	if len(nonEmptyParts) == 0 {
		return ""
	}
	if len(nonEmptyParts) == 1 {
		return nonEmptyParts[0]
	}
	return fmt.Sprintf("%s %v", nonEmptyParts[0], nonEmptyParts[1:])
}
