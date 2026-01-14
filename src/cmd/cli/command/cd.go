package command

import (
	"errors"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
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
			os.Setenv("DEFANG_JSON", "1") // FIXME: ugly way to set this globally
			global.Verbose = true
		}
	},
}

func cdCommand(cmd *cobra.Command, args []string, command client.CdCommand, fabric client.FabricClient) error {
	ctx := cmd.Context()
	session, err := NewCommandSession(cmd)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		projectName, err := client.LoadProjectNameWithFallback(ctx, session.Loader, session.Provider)
		if err != nil {
			return err
		}
		args = []string{projectName}
	}

	var errs []error
	for _, projectName := range args {
		err := canIUseProvider(ctx, session.Provider, projectName, 0)
		if err != nil {
			return err
		}
		errs = append(errs, cli.CdCommandAndTail(ctx, session.Provider, projectName, global.Verbose, command, fabric))
	}
	return errors.Join(errs...)
}

var cdDestroyCmd = &cobra.Command{
	Use:         "destroy [PROJECT...]",
	Annotations: authNeededAnnotation, // need subscription
	Short:       "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cdCommand(cmd, args, client.CdCommandDestroy, global.Client)
	},
}

var cdDownCmd = &cobra.Command{
	Use:         "down [PROJECT...]",
	Annotations: authNeededAnnotation, // need subscription
	Short:       "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cdCommand(cmd, args, client.CdCommandDown, global.Client)
	},
}

var cdRefreshCmd = &cobra.Command{
	Use:         "refresh [PROJECT...]",
	Annotations: authNeededAnnotation, // need subscription
	Short:       "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cdCommand(cmd, args, client.CdCommandRefresh, global.Client)
	},
}

var cdCancelCmd = &cobra.Command{
	Use:         "cancel [PROJECT...]",
	Annotations: authNeededAnnotation, // need subscription
	Short:       "Cancel the current CD operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cdCommand(cmd, args, client.CdCommandCancel, global.Client)
	},
}

var cdOutputsCmd = &cobra.Command{
	Use:         "outputs [PROJECT...]",
	Annotations: authNeededAnnotation, // need subscription
	Short:       "Get the outputs of the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cdCommand(cmd, args, client.CdCommandOutputs, global.Client)
	},
}

var cdTearDownCmd = &cobra.Command{
	Use:   "teardown",
	Args:  cobra.NoArgs,
	Short: "Destroy the CD cluster without destroying the services",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		force, _ := cmd.Flags().GetBool("force")

		session, err := NewCommandSession(cmd)
		if err != nil {
			return err
		}

		return cli.TearDownCD(ctx, session.Provider, force)
	},
}

var cdListCmd = &cobra.Command{
	Use:     "ls",
	Args:    cobra.NoArgs,
	Aliases: []string{"list"},
	Short:   "List all the projects and stacks in the CD cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		remote, _ := cmd.Flags().GetBool("remote")
		all, _ := cmd.Flags().GetBool("all")

		session, err := NewCommandSession(cmd)
		if err != nil {
			return err
		}

		if remote {
			if all {
				return errors.New("--all cannot be used with --remote")
			}

			err = canIUseProvider(cmd.Context(), session.Provider, "", 0)
			if err != nil {
				return err
			}

			// FIXME: this needs auth because it spawns the CD task
			return cli.CdCommandAndTail(cmd.Context(), session.Provider, "", global.Verbose, client.CdCommandList, global.Client)
		}
		return cli.CdListLocal(cmd.Context(), session.Provider, all)
	},
}

var cdPreviewCmd = &cobra.Command{
	Use:         "preview",
	Args:        cobra.NoArgs,
	Annotations: authNeededAnnotation, // FIXME: because it still needs a delegated domain
	Short:       "Preview the changes that will be made by the CD task",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		session, err := NewCommandSession(cmd)
		if err != nil {
			return err
		}
		project, err := session.Loader.LoadProject(cmd.Context())
		if err != nil {
			return err
		}

		err = canIUseProvider(ctx, session.Provider, project.Name, 1) // 1 SDU for BYOC preview
		if err != nil {
			return err
		}

		return cli.Preview(ctx, project, global.Client, session.Provider, cli.ComposeUpParams{
			Mode:       global.Stack.Mode,
			Project:    project,
			UploadMode: compose.UploadModePreview,
		})
	},
}

var cdInstallCmd = &cobra.Command{
	Use:         "install",
	Aliases:     []string{"setup"},
	Args:        cobra.NoArgs,
	Annotations: authNeededAnnotation,
	Short:       "Install the CD resources into the cluster",
	Hidden:      true, // users shouldn't have to run this manually, because it's done on deploy
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		session, err := NewCommandSession(cmd)
		if err != nil {
			return err
		}

		if err := canIUseProvider(ctx, session.Provider, "", 0); err != nil {
			return err
		}

		return cli.InstallCD(ctx, session.Provider)
	},
}

var cdCloudformationCmd = &cobra.Command{
	Use:         "cloudformation",
	Short:       "CloudFormation template related commands",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Hidden:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := aws.NewByocProvider(cmd.Context(), global.Client.GetTenantName(), global.Stack.Name)

		if err := canIUseProvider(cmd.Context(), provider, "", 0); err != nil {
			return err
		}

		template, err := provider.PrintCloudFormationTemplate()
		if err == nil {
			_, err = term.Print(string(template))
		}
		return err
	},
}
