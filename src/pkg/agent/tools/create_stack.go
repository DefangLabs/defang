package tools

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

type createStackParams struct {
	WorkingDirectory string
	Name             string
	Region           string
	Provider         client.ProviderID
	Mode             string
	Variables        map[string]string
}

func createStack(params createStackParams, sc StackConfig) (string, error) {
	if params.Mode == "" {
		params.Mode = modes.ModeAffordable.String()
	}
	mode, err := modes.Parse(params.Mode)
	if err != nil {
		return "Invalid mode provided", err
	}

	newStack := stacks.Parameters{
		Name:      params.Name,
		Region:    params.Region,
		Provider:  params.Provider,
		Mode:      mode,
		Variables: params.Variables,
	}

	_, err = stacks.CreateInDirectory(params.WorkingDirectory, newStack)
	if err != nil {
		return "Failed to create stack", err
	}

	err = stacks.LoadStackEnv(newStack, true)
	if err != nil {
		return "", fmt.Errorf("Unable to load stack %q: %w", newStack.Name, err)
	}

	*sc.Stack = newStack

	return fmt.Sprintf("Successfully created stack %q and loaded its environment.", newStack.Name), nil
}
