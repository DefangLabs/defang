package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/auth"
	cliPkg "github.com/DefangLabs/defang/src/pkg/cli"
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

	workingDir, err := loader.ProjectWorkingDir(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get project working directory: %w", err)
	}
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

	result, err := cliPkg.Cleanup(ctx, provider, ec, projectName, cliPkg.CleanupOptions{})
	if err != nil {
		if errors.Is(err, cliPkg.ErrCleanupUnsupported) {
			return "Resource cleanup is not supported for the selected provider.", nil
		}
		return "", err
	}
	if result.Found == 0 {
		return fmt.Sprintf("No leftover resources found for project %q.", projectName), nil
	}

	report := result.Report
	if result.ReportOnly {
		report += "\nRe-run this tool in an interactive session to apply these changes, then run `defang down` so Pulumi can finish removing the resources."
	} else if result.Cleaned > 0 {
		report += " Run `defang down` (or the destroy tool) again so Pulumi can finish removing any resources it was previously blocked from deleting."
	}
	return report, nil
}
