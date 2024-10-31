package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func BootstrapCommand(ctx context.Context, loader compose.Loader, client client.FabricClient, provider client.Provider, command string) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		// Some CD commands don't require a project name, so we don't return an error here.
		term.Debug("Failed to load project name:", err)
	}

	delegateDomain, err := client.GetDelegateSubdomainZone(ctx)
	if err != nil {
		term.Debug("Failed to get delegate domain:", err)
	}

	term.Debugf("Running CD command %s in project %q", command, projectName)
	if DoDryRun {
		return ErrDryRun
	}

	since := time.Now()
	etag, err := provider.BootstrapCommand(ctx, projectName, delegateDomain.Zone, command)
	if err != nil || etag == "" {
		return err
	}

	return tail(ctx, provider, TailOptions{Etag: etag, Since: since})
}

func BootstrapLocalList(ctx context.Context, provider client.Provider) error {
	term.Debug("Running CD list")
	if DoDryRun {
		return ErrDryRun
	}

	stacks, err := provider.BootstrapList(ctx)
	if err != nil {
		return err
	}
	for _, stack := range stacks {
		fmt.Println(" -", stack)
	}
	return nil
}
