package cli

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

// CleanupOptions controls how Cleanup resolves confirmation for each orphan resource found.
type CleanupOptions struct {
	// AutoConfirm skips the interactive prompt and cleans up every orphan found.
	AutoConfirm bool
	// ReportOnly finds and reports orphans without prompting or cleaning anything up. It is
	// ignored when AutoConfirm is set. Used when there is no TTY to prompt on.
	ReportOnly bool
}

// Cleanup finds AWS resources left behind by `defang down` that block Pulumi from finishing
// cleanup (see client.OrphanCleaner), then either reports them (ReportOnly) or unblocks each
// one — after an interactive confirmation, unless AutoConfirm is set. It writes progress to
// term as it goes and returns a final one-line summary.
func Cleanup(ctx context.Context, cleaner client.OrphanCleaner, projectName string, opts CleanupOptions) (string, error) {
	orphans, err := cleaner.DiscoverOrphans(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to discover leftover resources: %w", err)
	}
	if len(orphans) == 0 {
		return fmt.Sprintf("No leftover resources found for project %q that are blocking cleanup.", projectName), nil
	}

	term.Infof("Found %d leftover resource(s) for project %q blocking cleanup:", len(orphans), projectName)

	if opts.ReportOnly && !opts.AutoConfirm {
		for _, o := range orphans {
			term.Printf("- [%s] %s — would %s\n", o.Category, o.Name, o.Action)
		}
		return "Re-run with --yes, or in an interactive session, to apply these changes, then run 'defang down' so Pulumi can finish removing the resources.", nil
	}

	var cleaned, skipped, failed int
	for _, o := range orphans {
		confirm := opts.AutoConfirm
		if !opts.AutoConfirm {
			if err := survey.AskOne(&survey.Confirm{
				Message: fmt.Sprintf("Cleanup will %s for %s %q. Proceed?", o.Action, o.Category, o.Name),
			}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
				return "", err
			}
		}
		if !confirm {
			skipped++
			term.Printf("- [%s] %s — skipped\n", o.Category, o.Name)
			continue
		}
		if err := cleaner.CleanupOrphan(ctx, o); err != nil {
			failed++
			term.Printf("- [%s] %s — failed: %v\n", o.Category, o.Name, err)
			continue
		}
		cleaned++
		term.Printf("- [%s] %s — done (%s)\n", o.Category, o.Name, o.Action)
	}

	summary := fmt.Sprintf("%d cleaned, %d skipped, %d failed.", cleaned, skipped, failed)
	if cleaned > 0 {
		summary += " Run 'defang down' so Pulumi can finish removing the now-unblocked resources."
	}
	return summary, nil
}
