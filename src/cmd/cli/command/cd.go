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
		var utc, _ = cmd.Flags().GetBool("utc")
		var json, _ = cmd.Flags().GetBool("json")

		if utc {
			cli.EnableUTCMode()
		}

		if json {
			os.Setenv("DEFANG_JSON", "1")
			verbose = true
		}
	},
}

var cdDestroyCmd = &cobra.Command{
	Use:         "destroy",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, projectName)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), projectName, verbose, provider, "destroy")
	},
}

var cdDownCmd = &cobra.Command{
	Use:         "down",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, projectName)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), projectName, verbose, provider, "down")
	},
}

var cdRefreshCmd = &cobra.Command{
	Use:         "refresh",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, projectName)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), projectName, verbose, provider, "refresh")
	},
}

var cdCancelCmd = &cobra.Command{
	Use:         "cancel",
	Annotations: authNeededAnnotation, // need subscription
	Args:        cobra.NoArgs,         // TODO: set MaximumNArgs(1),
	Short:       "Cancel the current CD operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, projectName)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), projectName, verbose, provider, "cancel")
	},
}

var cdTearDownCmd = &cobra.Command{
	Use:   "teardown",
	Args:  cobra.NoArgs,
	Short: "Destroy the CD cluster without destroying the services",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		return cli.TearDown(cmd.Context(), provider, force)
	},
}

var cdListCmd = &cobra.Command{
	Use:     "ls",
	Args:    cobra.NoArgs,
	Aliases: []string{"list"},
	Short:   "List all the projects and stacks in the CD cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		remote, _ := cmd.Flags().GetBool("remote")

		provider, err := getProvider(cmd.Context(), nil)
		if err != nil {
			return err
		}

		if remote {
			err = canIUseProvider(cmd.Context(), provider, "")
			if err != nil {
				return err
			}

			// FIXME: this needs auth because it spawns the CD task
			return cli.BootstrapCommand(cmd.Context(), "", verbose, provider, "list")
		}
		return cli.BootstrapLocalList(cmd.Context(), provider)
	},
}

var cdPreviewCmd = &cobra.Command{
	Use:         "preview",
	Args:        cobra.NoArgs,
	Annotations: authNeededAnnotation, // FIXME: because it still needs a delegated domain
	Short:       "Preview the changes that will be made by the CD task",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		project, err := loader.LoadProject(cmd.Context())
		if err != nil {
			return err
		}

		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, project.Name)
		if err != nil {
			return err
		}

		return cli.Preview(cmd.Context(), project, client, provider, mode.Value())
	},
}
