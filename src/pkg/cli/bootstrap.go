package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func BootstrapCommand(ctx context.Context, loader client.Loader, c client.FabricClient, p client.Provider, cmd string) error {
	projectName, err := LoadProjectName(ctx, loader, p)
	if err != nil {
		// Some CD commands don't require a project name, so we don't return an error here.
		term.Debug("Failed to load project name:", err)
	}

	term.Debugf("Running CD command %s in project %q", cmd, projectName)
	if DoDryRun {
		return ErrDryRun
	}

	since := time.Now()
	etag, err := p.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: projectName, Command: cmd})
	if err != nil || etag == "" {
		return err
	}

	return tail(ctx, p, TailOptions{Project: projectName, Etag: etag, Since: since})
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
	if len(stacks) == 0 {
		fmt.Println("No projects found.")
	}
	for _, stack := range stacks {
		fmt.Println(" -", stack)
	}
	return nil
}
