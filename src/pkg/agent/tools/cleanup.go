package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type CleanupParams struct {
	common.LoaderParams
}

func HandleCleanupTool(ctx context.Context, loader client.Loader, params CleanupParams, cli CLIInterface, ec elicitations.Controller, sc StackConfig) (string, error) {
	term.Debug("Function invoked: cli.Connect")
	fabric, err := GetClientWithRetry(ctx, cli, sc.FabricAddr)
	if err != nil {
		var noBrowserErr auth.ErrNoBrowser
		if errors.As(err, &noBrowserErr) {
			return noBrowserErr.Error(), nil
		}
		return "", err
	}

	workingDir, _ := loader.ProjectWorkingDir(ctx)
	sm, err := stacks.NewManager(fabric, workingDir, params.ProjectName, ec)
	if err != nil {
		return "", fmt.Errorf("failed to create stack manager: %w", err)
	}
	pp := NewProviderPreparer(cli, ec, fabric, sm)
	_, provider, err := pp.SetupProvider(ctx, sc.Stack)
	if err != nil {
		return "", fmt.Errorf("failed to setup provider: %w", err)
	}

	projectName, err := cli.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return "", fmt.Errorf("failed to load project name: %w", err)
	}

	if err := cli.CanIUseProvider(ctx, fabric, provider, projectName, 0); err != nil {
		return "", fmt.Errorf("failed to use provider: %w", err)
	}

	cleaner, ok := provider.(client.OrphanCleaner)
	if !ok {
		return "Resource cleanup is currently only supported for AWS. The selected provider does not retain resources that need manual cleanup.", nil
	}

	orphans, err := cleaner.DiscoverOrphans(ctx, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to discover leftover resources: %w", err)
	}
	if len(orphans) == 0 {
		return fmt.Sprintf("No leftover resources found for project %q that are blocking cleanup.", projectName), nil
	}

	var report strings.Builder
	fmt.Fprintf(&report, "Found %d leftover resource(s) for project %q blocking cleanup:\n", len(orphans), projectName)

	// Without interactive elicitation we cannot get confirmation for these destructive actions,
	// so only report what was found and let the caller decide.
	if !ec.IsSupported() {
		for _, o := range orphans {
			fmt.Fprintf(&report, "- [%s] %s — would %s\n", o.Category, o.Name, o.Action)
		}
		report.WriteString("\nRe-run this tool in an interactive session to apply these changes, then run `defang down` so Pulumi can finish removing the resources.")
		return report.String(), nil
	}

	var cleaned, skipped, failed int
	for _, o := range orphans {
		confirm, err := ec.RequestEnum(ctx,
			fmt.Sprintf("Cleanup will %s for %s %q. Proceed?", o.Action, o.Category, o.Name),
			"confirm", []string{"no", "yes"})
		if err != nil {
			return "", fmt.Errorf("failed to confirm cleanup: %w", err)
		}
		if confirm != "yes" {
			skipped++
			fmt.Fprintf(&report, "- [%s] %s — skipped\n", o.Category, o.Name)
			continue
		}
		if err := cleaner.CleanupOrphan(ctx, o); err != nil {
			failed++
			fmt.Fprintf(&report, "- [%s] %s — failed: %v\n", o.Category, o.Name, err)
			continue
		}
		cleaned++
		fmt.Fprintf(&report, "- [%s] %s — done (%s)\n", o.Category, o.Name, o.Action)
	}

	fmt.Fprintf(&report, "\n%d cleaned, %d skipped, %d failed.", cleaned, skipped, failed)
	if cleaned > 0 {
		report.WriteString(" Run `defang down` (or the destroy tool) so Pulumi can finish removing the now-unblocked resources.")
	}
	return report.String(), nil
}
