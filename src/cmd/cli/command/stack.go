package command

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli"
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
			ctx := cmd.Context()

			var stackName string
			if len(args) > 0 {
				stackName = args[0]
				if err := stacks.ValidateStackName(stackName); err != nil {
					return fmt.Errorf("invalid stack name %q: %v", stackName, err)
				}
			}

			loader := configureLoaderForCommand(cmd)
			projectName, _, err := loader.LoadProjectName(ctx)
			if err != nil {
				return err
			}

			if stackName != "" {
				exists, err := stackExists(ctx, projectName, stackName)
				if err != nil {
					return err
				}
				if exists {
					return fmt.Errorf("stack with name %q already exists in project %q", stackName, projectName)
				}
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

			err = PromptForStackParameters(ctx, &params)
			if err != nil {
				return err
			}

			exists, err := stackExists(ctx, projectName, params.Name)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("stack with name %q already exists in project %q", params.Name, projectName)
			}

			term.Debugf("Creating stack with parameters: %+v\n", params)

			_, err = stacks.CreateInDirectory(".", params)
			if err != nil {
				return err
			}

			stacks.PrintCreateMessage(params.Name)
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
			ctx := cmd.Context()
			loader := configureLoaderForCommand(cmd)
			projectName, _, err := loader.LoadProjectName(ctx)
			if err != nil {
				return err
			}

			workingDir, _ := loader.ProjectWorkingDir(ctx)
			sm, err := stacks.NewManager(global.Client, workingDir, projectName, ec)
			if err != nil {
				return err
			}

			stackList, err := sm.List(ctx)
			if err != nil {
				return err
			}

			filteredStacks := make([]stacks.ListItem, 0, len(stackList))
			for _, stack := range stackList {
				if stack.Status == defangv1.StackStatus_STACK_STATUS_DOWN {
					continue
				}
				filteredStacks = append(filteredStacks, stack)
			}

			if len(filteredStacks) == 0 {
				_, err = term.Infof("No Defang stacks found in the current directory.\n")
				return err
			}

			columns := []string{"Name", "Default", "Provider", "Region", "Account", "Mode", "DeployedAt"}
			return term.Table(filteredStacks, columns...)
		},
	}
	stackListCmd.Flags().Bool("json", false, "Output in JSON format")
	return stackListCmd
}

func makeStackDefaultCmd() *cobra.Command {
	var stackDefaultCmd = &cobra.Command{
		Use:     "default STACK_NAME",
		Aliases: []string{"set-default", "select", "use"},
		Args:    cobra.ExactArgs(1),
		Short:   "Set the default Defang deployment stack for the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			loader := configureLoaderForCommand(cmd)
			projectName, _, err := loader.LoadProjectName(ctx)
			if err != nil {
				return err
			}

			workingDir, _ := loader.ProjectWorkingDir(ctx)
			sm, err := stacks.NewManager(global.Client, workingDir, projectName, ec)
			if err != nil {
				return err
			}

			err = cli.SetDefaultStack(ctx, global.Client, sm, projectName, name)
			if err != nil {
				return err
			}

			term.Info(fmt.Sprintf("Stack %q is now the default stack for project %q\n", name, projectName))
			return nil
		},
	}
	return stackDefaultCmd
}

func makeStackRemoveCmd() *cobra.Command {
	var force bool
	var stackRemoveCmd = &cobra.Command{
		Use:     "remove STACK_NAME",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		Short:   "Remove an existing Defang deployment stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			loader := configureLoaderForCommand(cmd)
			sm, err := newStackManagerForLoader(ctx, loader)
			if err != nil {
				return err
			}
			projectName, _, err := loader.LoadProjectName(ctx)
			if err != nil {
				return err
			}

			stack, err := sm.Load(ctx, name)
			if err != nil {
				return fmt.Errorf("could not load stack parameters: %w", err)
			}
			provider := cli.NewProvider(ctx, stack.Provider, global.Client, stack.Name)
			return cli.RemoveStack(ctx, global.Client, provider, ec, projectName, name, force)
		},
	}
	stackRemoveCmd.Flags().BoolVarP(&force, "force", "", false, "Force removal of the stack even if it has active deployments")
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

func stackExists(ctx context.Context, project string, stack string) (bool, error) {
	if stack == "" {
		return false, nil
	}
	resp, err := global.Client.GetStack(ctx, &defangv1.GetStackRequest{
		Project: project,
		Stack:   stack,
	})
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	return resp.GetStack() != nil, nil
}
