package command

import (
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/debug"
	"github.com/DefangLabs/defang/src/pkg/session"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:         "cleanup",
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "Find and remove cloud resources left behind after a down",
	Long: `Find and remove the cloud resources that remain after "defang down".

Some resources are deliberately retained by the deployment, and some cannot be deleted while
another resource still references them. Both cases leave resources behind and can eventually hit
a cloud quota. This command finds them and performs the minimum action needed for each, asking
before every change.

On AWS this disables deletion protection, deletes leftover DNS records, and empties container
repositories, so that a following "defang down" can complete. On GCP it removes the retained VPC
networking. On Azure it removes the project resource group.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		yes, _ := cmd.Flags().GetBool("yes")

		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}

		projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
		if err != nil {
			return err
		}

		if err := canIUseProvider(cmd.Context(), session.Provider, projectName, 0, false); err != nil {
			return err
		}

		term.Infof("Looking for leftover resources for project %q…", projectName)
		result, err := cli.Cleanup(cmd.Context(), session.Provider, ec, projectName, cli.CleanupOptions{
			DryRun: dryRun,
			Yes:    yes,
		})
		if err != nil {
			if errors.Is(err, cli.ErrCleanupUnsupported) {
				// Not every provider has anything to clean up; that is not a command failure.
				term.Warnf("Cleanup is not supported for provider %q.", session.Stack.Provider)
				return nil
			}
			return err
		}

		if result.Found == 0 {
			term.Info("No leftover resources found.")
			return nil
		}

		term.Println(result.Report)

		if result.ReportOnly {
			if dryRun {
				printDefangHint("To remove these resources, run:", "cleanup")
			} else {
				// Confirmation is impossible here, so the run listed rather than deleted.
				term.Warn("Cannot confirm destructive actions in non-interactive mode; nothing was changed.")
				printDefangHint("To remove these resources without confirmation, run:", "cleanup --yes")
			}
			return nil
		}

		if result.Cleaned > 0 {
			printDefangHint("To let the deployment finish removing what it was blocked on, run:", "down")
		}
		if result.Failed > 0 {
			// A failed cleanup is exactly the case the AI debugger is useful for: it can read the
			// error, work out which resource is still holding the one that failed, and use the
			// cleanup_resources tool itself. offerCleanupDebugger skips the prompt when
			// non-interactive.
			offerCleanupDebugger(cmd, session, result)
			return fmt.Errorf("%d resource(s) could not be cleaned up", result.Failed)
		}
		return nil
	},
}

// offerCleanupDebugger hands a failed cleanup to the AI debugger. A cleanup fails when a resource
// is still referenced by something this command did not find, which is a diagnosis job: the agent
// can read the error, look at what is holding the resource, and call the cleanup_resources tool
// itself. Any failure to start the debugger is reported as a warning, because the caller is
// already returning the cleanup error.
func offerCleanupDebugger(cmd *cobra.Command, session *session.Session, result cli.CleanupResult) {
	debugConfig := debug.DebugConfig{
		Operation:  debug.OperationCleanup,
		ProviderID: &session.Stack.Provider,
		Stack:      session.Stack.Name,
	}
	// There is no one to prompt in CI, so print a hint instead of building the debugger, matching
	// how a failed deployment behaves.
	if global.NonInteractive {
		printDefangHint("To debug the failed cleanup, do:", debugConfig.String())
		return
	}
	debugger, err := debug.NewDebugger(cmd.Context(), global.FabricAddr, session.Stack, true)
	if err != nil {
		term.Debugf("Failed to initialize debugger: %v", err)
		printDefangHint("To debug the failed cleanup, do:", debugConfig.String())
		return
	}
	if err := debugger.DebugCleanupError(cmd.Context(), debugConfig, errors.New(result.Report)); err != nil {
		term.Debugf("Debugger failed: %v", err)
	}
}
