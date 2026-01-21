package command

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

func makeStackCmd() *cobra.Command {
	var stackCmd = &cobra.Command{
		Use:     "stack",
		Aliases: []string{"stacks"},
		Short:   "Manage Defang deployment stacks",
	}
	stackNewCmd := makeStackNewCmd()
	stackCmd.AddCommand(stackNewCmd)
	stackListCmd := makeStackListCmd()
	stackCmd.AddCommand(stackListCmd)
	stackCmd.AddCommand(makeStackDefaultCmd())
	stackRemoveCmd := makeStackRemoveCmd()
	stackRemoveCmd.Hidden = true
	stackCmd.AddCommand(stackRemoveCmd)
	return stackCmd
}

func makeStackNewCmd() *cobra.Command {
	var stackNewCmd = &cobra.Command{
		Use:     "new [STACK_NAME]",
		Aliases: []string{"init", "create"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Create a new Defang deployment stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			var stackName string
			if len(args) > 0 {
				stackName = args[0]
			}

			var region, _ = cmd.Flags().GetString("region")

			params := stacks.Parameters{
				Name:     stackName,
				Provider: global.Stack.Provider, // default provider
				Region:   region,
				Mode:     global.Stack.Mode,
			}

			if global.NonInteractive {
				_, err := stacks.CreateInDirectory(".", params)
				return err
			}

			ctx := cmd.Context()
			err := PromptForStackParameters(ctx, &params)
			if err != nil {
				return err
			}

			term.Debugf("Creating stack with parameters: %+v\n", params)

			_, err = stacks.CreateInDirectory(".", params)
			if err != nil {
				return err
			}

			term.Info(stacks.PostCreateMessage(params.Name))
			return nil
		},
	}
	stackNewCmd.Flags().VarP(&global.Stack.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	stackNewCmd.Flags().StringVarP(&global.Stack.Region, "region", "r", global.Stack.Region, "Cloud region for the stack deployment")

	return stackNewCmd
}

func makeStackListCmd() *cobra.Command {
	stackListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Short:   "List existing Defang deployment stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")

			ctx := cmd.Context()
			loader := configureLoader(cmd)
			projectName, _, err := loader.LoadProjectName(ctx)
			if err != nil {
				return err
			}

			sm, err := stacks.NewManager(global.Client, loader.TargetDirectory(), projectName, ec)
			if err != nil {
				return err
			}

			stacks, err := sm.List(ctx)
			if err != nil {
				return err
			}

			if jsonMode {
				jsonData := []byte("[]")
				if len(stacks) > 0 {
					jsonData, err = json.MarshalIndent(stacks, "", "  ")
					if err != nil {
						return err
					}
				}

				_, err = term.Print(string(jsonData) + "\n")
				return err
			}

			if len(stacks) == 0 {
				_, err = term.Infof("No Defang stacks found in the current directory.\n")
				return err
			}

			columns := []string{"Name", "Provider", "Region", "Mode", "DeployedAt"}
			return term.Table(stacks, columns...)
		},
	}
	stackListCmd.Flags().Bool("json", false, "Output in JSON format")
	return stackListCmd
}

func makeStackDefaultCmd() *cobra.Command {
	var stackDefaultCmd = &cobra.Command{
		Use:     "default STACK_NAME",
		Aliases: []string{"set-default"},
		Args:    cobra.ExactArgs(1),
		Short:   "Set the default Defang deployment stack for the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			loader := configureLoader(cmd)
			projectName, _, err := loader.LoadProjectName(ctx)
			if err != nil {
				return err
			}

			sm, err := stacks.NewManager(global.Client, loader.TargetDirectory(), projectName, ec)
			if err != nil {
				return err
			}

			stack, err := sm.Load(ctx, name) // verify stack exists
			if err != nil {
				return err
			}

			stackfile, err := stacks.Marshal(stack)
			if err != nil {
				return err
			}

			err = global.Client.PutStack(ctx, &defangv1.PutStackRequest{
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
		},
	}
	return stackDefaultCmd
}

func makeStackRemoveCmd() *cobra.Command {
	var stackRemoveCmd = &cobra.Command{
		Use:     "remove STACK_NAME",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		Short:   "Remove an existing Defang deployment stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return stacks.RemoveInDirectory(".", name)
		},
	}
	return stackRemoveCmd
}

func PromptForStackParameters(ctx context.Context, params *stacks.Parameters) error {
	wizard := stacks.NewWizard(ec)
	newParams, err := wizard.CollectRemainingParameters(ctx, params)
	if err != nil {
		return err
	}

	*params = *newParams

	return nil
}
