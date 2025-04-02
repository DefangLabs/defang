package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func BootstrapCommand(ctx context.Context, projectName string, verbose bool, p client.Provider, cmd string) error {
	term.Infof("Running CD command %q in project %q", cmd, projectName)
	if DoDryRun {
		return ErrDryRun
	}

	since := time.Now()
	etag, err := p.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: projectName, Command: cmd})
	if err != nil || etag == "" {
		return err
	}

	return tail(ctx, p, projectName, TailOptions{Deployment: etag, Since: since, LogType: logs.LogTypeBuild, Verbose: verbose})
}

func SplitProjectStack(name string) (projectName string, stackName string) {
	parts := strings.SplitN(name, "/", 2)
	return parts[0], parts[1]
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
		accountInfo, err := provider.AccountInfo(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("No projects found for account '%s' at region '%s'\n", accountInfo.AccountID(), accountInfo.Region())
	}

	for _, stack := range stacks {
		projectName, _ := SplitProjectStack(stack)
		fmt.Println(" -", projectName)
	}

	return nil
}
