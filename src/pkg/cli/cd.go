package cli

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

func CdCommand(ctx context.Context, projectName string, provider client.Provider, fabric client.FabricClient, command client.CdCommand) (types.ETag, error) {
	if projectName == "" { // projectName is empty for "list --remote"
		term.Infof("Running CD command %q", command)
	} else {
		term.Infof("Running CD command %q in project %q", command, projectName)
	}
	if dryrun.DoDryRun {
		return "", dryrun.ErrDryRun
	}

	etag, err := provider.CdCommand(ctx, client.CdCommandRequest{Project: projectName, Command: command})
	if err != nil || etag == "" {
		return "", err
	}

	if command == client.CdCommandDestroy || command == client.CdCommandDown {
		err := postDown(ctx, projectName, provider, fabric, etag)
		if err != nil {
			term.Debugf("postDown failed: %v", err)
			term.Warn("Unable to update deployment history; deployment will proceed anyway.")
		}
	}
	return etag, nil
}

func postDown(ctx context.Context, projectName string, provider client.Provider, fabric client.FabricClient, etag types.ETag) error {
	// Special bookkeeping for "down" commands: delete the subdomain zone and mark deployment as destroyed
	err := fabric.DeleteSubdomainZone(ctx, &defangv1.DeleteSubdomainZoneRequest{
		Project: projectName,
		Stack:   provider.GetStackNameForDomain(),
	})
	if err != nil {
		// This can fail when the project was deployed from a different workspace than the current one
		term.Debug("DeleteSubdomainZone failed:", err)
		if connect.CodeOf(err) == connect.CodeNotFound {
			term.Warn("Subdomain not found; did you mean to destroy a different project or stack?")
		}
		return err
	}

	// Update deployment table to mark deployment as destroyed only after successful deletion of the subdomain
	err = putDeployment(ctx, provider, fabric, putDeploymentParams{
		Action:      defangv1.DeploymentAction_DEPLOYMENT_ACTION_DOWN,
		ETag:        etag,
		ProjectName: projectName,
	})
	if err != nil {
		term.Debug("Failed to record deployment:", err)
		return err
	}
	return nil
}

func CdCommandAndTail(ctx context.Context, provider client.Provider, projectName string, verbose bool, command client.CdCommand, fabric client.FabricClient) error {
	since := time.Now()
	etag, err := CdCommand(ctx, projectName, provider, fabric, command)
	if err != nil {
		return err
	}

	options := TailOptions{
		Deployment: etag,
		LogType:    logs.LogTypeBuild,
		Since:      since,
		Verbose:    verbose,
	}
	return TailAndWaitForCD(ctx, provider, projectName, options)
}

func TailAndWaitForCD(ctx context.Context, provider client.Provider, projectName string, tailOptions TailOptions) error {
	tailOptions.Follow = true
	ctx, cancelTail := context.WithCancelCause(ctx)
	defer cancelTail(nil) // to cancel tail and clean-up context

	var cdErr error
	go func() {
		cdErr = WaitForCdTaskExit(ctx, provider)
		pkg.SleepWithContext(ctx, 2*time.Second) // a delay before cancelling tail to make sure we got the last logs
		cancelTail(cdErr)
	}()

	// blocking call to tail
	var tailErr error
	if err := streamLogs(ctx, provider, projectName, tailOptions, logEntryPrintHandler); err != nil {
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

func CdListLocal(ctx context.Context, provider client.Provider, allRegions bool) error {
	term.Debug("Running CD list")
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	stacks, err := provider.CdList(ctx, allRegions)
	if err != nil {
		return err
	}

	var count int
	for stack := range stacks {
		count++
		if !allRegions {
			stack, _ = SplitProjectStack(stack)
		}
		term.Println(" -", stack) // TODO: json output mode
	}
	if count == 0 {
		accountInfo, err := provider.AccountInfo(ctx)
		if err != nil {
			return err
		}
		if allRegions {
			accountInfo.Region = ""
		}
		term.Printf("No projects found in %v\n", accountInfo)
	}
	return nil
}
