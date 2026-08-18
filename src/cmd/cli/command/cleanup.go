package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:         "cleanup",
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "Find and unblock AWS resources left behind after 'defang down'",
	Long: `Find and unblock AWS resources left behind after 'defang down' that prevent Pulumi
from finishing cleanup: load balancers and databases with deletion protection enabled,
leftover Route53 records, and non-empty ECR repositories. For each one found, this
performs the minimum action needed to unblock it (disable deletion protection, delete
the record, delete the images) and nothing more, so it never deletes an artifact or
secret that 'defang down' itself would not have deleted.

Only AWS is supported today. After running, run 'defang down' again so Pulumi can
finish removing the now-unblocked resources.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		allowUpgrade, _ := cmd.Flags().GetBool("allow-upgrade")
		yes, _ := cmd.Flags().GetBool("yes")

		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}

		projectName, err := client.LoadProjectNameWithFallback(ctx, session.Loader, session.Provider)
		if err != nil {
			return err
		}

		if err := canIUseProvider(ctx, session.Provider, projectName, 0, allowUpgrade); err != nil {
			return err
		}

		cleaner, ok := session.Provider.(client.OrphanCleaner)
		if !ok {
			term.Info("Resource cleanup is currently only supported for AWS. The selected provider does not retain resources that need manual cleanup.")
			return nil
		}

		summary, err := cli.Cleanup(ctx, cleaner, projectName, cli.CleanupOptions{
			AutoConfirm: yes,
			ReportOnly:  global.NonInteractive,
		})
		if err != nil {
			return err
		}
		term.Info(summary)
		return nil
	},
}
