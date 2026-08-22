package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
)

// ErrCleanupUnsupported is returned when the selected provider cannot find or remove leftover
// resources. OrphanCleaner is an optional capability, so this is a normal outcome, not a failure.
var ErrCleanupUnsupported = errors.New("resource cleanup is not supported for the selected provider")

// CleanupResult reports what a Cleanup run did, so callers can set an exit code or word a hint.
type CleanupResult struct {
	Found   int
	Cleaned int
	Skipped int
	Failed  int
	// Report is the human-readable log of the run, one line per resource.
	Report string
	// ReportOnly is true when the run only listed resources, because there was no way to ask for
	// confirmation (non-interactive) or the caller asked for a dry run.
	ReportOnly bool
}

// CleanupOptions controls a Cleanup run.
type CleanupOptions struct {
	// DryRun lists the leftover resources without changing anything.
	DryRun bool
	// Yes cleans up every resource without asking. Ignored when DryRun is set.
	Yes bool
}

// Cleanup finds the cloud resources a provider left behind after `defang down` and removes them,
// confirming each one first. It is shared by the `cleanup` command and the `cleanup_resources`
// agent tool so that both behave identically.
//
// Providers return their orphans in the order they must be cleaned up (GCP networking, for
// instance, has to go instance templates first and the network last), so this iterates in order
// and never reorders or parallelises.
func Cleanup(ctx context.Context, provider client.Provider, ec elicitations.Controller, projectName string, opts CleanupOptions) (CleanupResult, error) {
	cleaner, ok := provider.(client.OrphanCleaner)
	if !ok {
		return CleanupResult{}, ErrCleanupUnsupported
	}

	orphans, err := cleaner.DiscoverOrphans(ctx, projectName)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("failed to discover leftover resources: %w", err)
	}

	result := CleanupResult{Found: len(orphans)}
	if len(orphans) == 0 {
		return result, nil
	}

	var report strings.Builder
	fmt.Fprintf(&report, "Found %d leftover resource(s) for project %q:\n", len(orphans), projectName)

	// Without interactive elicitation there is no way to confirm these destructive actions, so
	// only report what was found and let the caller decide what to do about it.
	if opts.DryRun || (!opts.Yes && !ec.IsSupported()) {
		for _, o := range orphans {
			fmt.Fprintf(&report, "- [%s] %s — would %s\n", o.Category, o.Name, o.Action)
		}
		result.ReportOnly = true
		result.Report = report.String()
		return result, nil
	}

	for _, o := range orphans {
		if !opts.Yes {
			confirm, err := ec.RequestEnum(ctx,
				fmt.Sprintf("Cleanup will %s for %s %q. Proceed?", o.Action, o.Category, o.Name),
				"confirm", []string{"no", "yes"})
			if err != nil {
				result.Report = report.String()
				return result, fmt.Errorf("failed to confirm cleanup: %w", err)
			}
			if confirm != "yes" {
				result.Skipped++
				fmt.Fprintf(&report, "- [%s] %s — skipped\n", o.Category, o.Name)
				continue
			}
		}
		if err := cleaner.CleanupOrphan(ctx, o); err != nil {
			result.Failed++
			fmt.Fprintf(&report, "- [%s] %s — failed: %v\n", o.Category, o.Name, err)
			continue
		}
		result.Cleaned++
		fmt.Fprintf(&report, "- [%s] %s — done (%s)\n", o.Category, o.Name, o.Action)
	}

	fmt.Fprintf(&report, "\n%d cleaned, %d skipped, %d failed.", result.Cleaned, result.Skipped, result.Failed)
	result.Report = report.String()
	return result, nil
}
