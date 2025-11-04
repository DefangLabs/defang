package command

import (
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/spf13/cobra"
)

var cdCmd = &cobra.Command{
	Use:     "cd",
	Aliases: []string{"bootstrap", "pulumi"},
	Args:    cobra.NoArgs,
	Short:   "Manually run a command with the CD task (for BYOC only)",
	Hidden:  true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()
		var utc, _ = cmd.Flags().GetBool("utc")
		var json, _ = cmd.Flags().GetBool("json")

		if utc {
			cli.EnableUTCMode()
		}

		if json {
			os.Setenv("DEFANG_JSON", "1")
			ctx = withVerbose(ctx, true)
			cmd.SetContext(ctx)
		}
	},
}

var cdDestroyCmd = &cobra.Command{
	Use:         "destroy",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localVerbose := getVerbose(ctx)
		localNonInteractive := getNonInteractive(ctx)

		loader := configureLoader(cmd, localNonInteractive)
		provider, err := newProviderChecked(ctx, loader, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(ctx, localClient, provider, projectName, localStack, 0)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(ctx, projectName, localVerbose, provider, "destroy")
	},
}

var cdDownCmd = &cobra.Command{
	Use:         "down",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localVerbose := getVerbose(ctx)
		localNonInteractive := getNonInteractive(ctx)

		loader := configureLoader(cmd, localNonInteractive)
		provider, err := newProviderChecked(ctx, loader, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(ctx, localClient, provider, projectName, localStack, 0)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(ctx, projectName, localVerbose, provider, "down")
	},
}

var cdRefreshCmd = &cobra.Command{
	Use:         "refresh",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localVerbose := getVerbose(ctx)
		localNonInteractive := getNonInteractive(ctx)

		loader := configureLoader(cmd, localNonInteractive)
		provider, err := newProviderChecked(ctx, loader, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(ctx, localClient, provider, projectName, localStack, 0)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(ctx, projectName, localVerbose, provider, "refresh")
	},
}

var cdCancelCmd = &cobra.Command{
	Use:         "cancel",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Cancel the current CD operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localVerbose := getVerbose(ctx)
		localNonInteractive := getNonInteractive(ctx)

		loader := configureLoader(cmd, localNonInteractive)
		provider, err := newProviderChecked(ctx, loader, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(ctx, localClient, provider, projectName, localStack, 0)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(ctx, projectName, localVerbose, provider, "cancel")
	},
}

var cdTearDownCmd = &cobra.Command{
	Use:   "teardown",
	Args:  cobra.NoArgs,
	Short: "Destroy the CD cluster without destroying the services",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localNonInteractive := getNonInteractive(ctx)

		force, _ := cmd.Flags().GetBool("force")

		loader := configureLoader(cmd, localNonInteractive)
		provider, err := newProviderChecked(ctx, loader, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		return cli.TearDown(ctx, provider, force)
	},
}

var cdListCmd = &cobra.Command{
	Use:     "ls",
	Args:    cobra.NoArgs,
	Aliases: []string{"list"},
	Short:   "List all the projects and stacks in the CD cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localVerbose := getVerbose(ctx)
		localNonInteractive := getNonInteractive(ctx)

		remote, _ := cmd.Flags().GetBool("remote")

		provider, err := newProviderChecked(ctx, nil, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		if remote {
			err = canIUseProvider(ctx, localClient, provider, "", localStack, 0)
			if err != nil {
				return err
			}

			// FIXME: this needs auth because it spawns the CD task
			return cli.BootstrapCommand(ctx, "", localVerbose, provider, "list")
		}
		return cli.BootstrapLocalList(ctx, provider)
	},
}

var cdPreviewCmd = &cobra.Command{
	Use:         "preview",
	Args:        cobra.NoArgs,
	Annotations: authNeededAnnotation, // FIXME: because it still needs a delegated domain
	Short:       "Preview the changes that will be made by the CD task",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		localClient := getClient(ctx)
		localProviderID := getProviderID(ctx)
		localStack := getStack(ctx)
		localMode := getMode(ctx)
		localNonInteractive := getNonInteractive(ctx)

		loader := configureLoader(cmd, localNonInteractive)
		project, err := loader.LoadProject(ctx)
		if err != nil {
			return err
		}

		provider, err := newProviderChecked(ctx, loader, localClient, localProviderID, localStack, localNonInteractive)
		if err != nil {
			return err
		}

		err = canIUseProvider(ctx, localClient, provider, project.Name, localStack, 1) // 1 SDU for BYOC preview
		if err != nil {
			return err
		}

		return cli.Preview(ctx, project, localClient, provider, localMode)
	},
}
