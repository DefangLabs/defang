package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

var cdCmd = &cobra.Command{
	Use:     "cd",
	Aliases: []string{"bootstrap"},
	Short:   "Manually run a command with the CD task (for BYOC only)",
	Hidden:  true,
}

var cdDestroyCmd = &cobra.Command{
	Use:         "destroy",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short:       "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), loader, client, provider, "destroy")
	},
}

var cdDownCmd = &cobra.Command{
	Use:         "down",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short:       "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), loader, client, provider, "down")
	},
}

var cdRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Args:  cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short: "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), loader, client, provider, "refresh")
	},
}

var cdCancelCmd = &cobra.Command{
	Use:   "cancel",
	Args:  cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short: "Cancel the current CD operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		return cli.BootstrapCommand(cmd.Context(), loader, client, provider, "cancel")
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

		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		if remote {
			// FIXME: this needs auth because it spawns the CD task
			return cli.BootstrapCommand(cmd.Context(), loader, client, provider, "list")
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

		resp, project, err := cli.ComposeUp(cmd.Context(), project, client, provider, compose.UploadModePreview, defangv1.DeploymentMode_UNSPECIFIED_MODE)
		if err != nil {
			return err
		}

		tailOptions := cli.TailOptions{
			Etag:    resp.Etag,
			Verbose: verbose,
			LogType: logs.LogTypeAll,
		}
		return cli.Tail(cmd.Context(), provider, project.Name, tailOptions)
	},
}
