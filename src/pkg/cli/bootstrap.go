package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func BootstrapCommand(ctx context.Context, projectName string, verbose bool, p client.Provider, cmd string) error {
	if projectName == "" { // projectName is empty for "list --remote"
		term.Infof("Running CD command %q", cmd)
	} else {
		term.Infof("Running CD command %q in project %q", cmd, projectName)
	}
	if DoDryRun {
		return ErrDryRun
	}

	since := time.Now()
	etag, err := p.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: projectName, Command: cmd})
	if err != nil || etag == "" {
		return err
	}

	options := TailOptions{
		Deployment: etag,
		Since:      since,
		LogType:    logs.LogTypeBuild,
		Verbose:    verbose,
	}
	return tailAndWaitForCD(ctx, projectName, p, options, LogEntryPrintHandler)
}

func tailAndWaitForCD(ctx context.Context, projectName string, provider client.Provider, tailOptions TailOptions, handler LogEntryHandler) error {
	ctx, cancelTail := context.WithCancelCause(ctx)
	defer cancelTail(nil) // to cancel tail and clean-up context

	var cdErr error
	go func() {
		cdErr = client.WaitForCdTaskExit(ctx, provider)
		pkg.SleepWithContext(ctx, 2*time.Second) // a delay before cancelling tail to make sure we got the last logs
		cancelTail(cdErr)
	}()

	// blocking call to tail
	var tailErr error
	if err := streamLogs(ctx, provider, projectName, tailOptions, handler); err != nil {
		term.Debug("Tail stopped with", err, errors.Unwrap(err))
		if !errors.Is(err, context.Canceled) {
			tailErr = err
		}
	}
	return errors.Join(cdErr, tailErr)
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
