package command

import (
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/spf13/cobra"
)

func makeRecipeCmd() *cobra.Command {
	var recipeCmd = &cobra.Command{
		Use:     "recipe",
		Aliases: []string{"recipes", "modes", "mode"},
		Short:   "Manage workspace recipes (deployment modes)",
	}
	recipeListCmd := makeRecipeListCmd()
	recipeCmd.AddCommand(recipeListCmd)
	recipeShowCmd := makeRecipeShowCmd()
	recipeCmd.AddCommand(recipeShowCmd)
	recipeDeactivateCmd := makeRecipeDeactivateCmd()
	recipeCmd.AddCommand(recipeDeactivateCmd)
	recipeActivateCmd := makeRecipeActivateCmd()
	recipeCmd.AddCommand(recipeActivateCmd)
	return recipeCmd
}

func makeRecipeShowCmd() *cobra.Command {
	var recipeShowCmd = &cobra.Command{
		Use:         "show [RECIPE_NAME]",
		Aliases:     []string{"get", "describe", "desc"},
		Annotations: authNeededAlways,
		Args:        cobra.ExactArgs(1),
		Short:       "Show details of a recipe in the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return cli.RecipeShow(ctx, global.Client, args[0])
		},
	}
	return recipeShowCmd
}

func makeRecipeListCmd() *cobra.Command {
	var recipeListCmd = &cobra.Command{
		Use:         "list",
		Aliases:     []string{"ls"},
		Annotations: authNeededAlways,
		Args:        cobra.NoArgs,
		Short:       "List recipes in the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return cli.RecipeList(ctx, global.Client)
		},
	}
	return recipeListCmd
}

func makeRecipeDeactivateCmd() *cobra.Command {
	var recipeArchiveCmd = &cobra.Command{
		Use:         "deactivate [RECIPE_NAME...]",
		Aliases:     []string{"remove", "rm", "delete", "del", "disable", "archive"},
		Annotations: authNeededAlways,
		Args:        cobra.MinimumNArgs(1),
		Short:       "Deactivates a recipe in the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var errs []error
			for _, name := range args {
				errs = append(errs, cli.RecipeActivate(ctx, global.Client, name, false))
			}
			return errors.Join(errs...)
		},
	}
	return recipeArchiveCmd
}

func makeRecipeActivateCmd() *cobra.Command {
	var recipeUnarchiveCmd = &cobra.Command{
		Use:         "activate [RECIPE_NAME...]",
		Aliases:     []string{"restore", "enable", "undelete", "unarchive"},
		Annotations: authNeededAlways,
		Args:        cobra.MinimumNArgs(1),
		Short:       "Activates a recipe in the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			var errs []error
			for _, name := range args {
				errs = append(errs, cli.RecipeActivate(ctx, global.Client, name, true))
			}
			return errors.Join(errs...)
		},
	}
	return recipeUnarchiveCmd
}
