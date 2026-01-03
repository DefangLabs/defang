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
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func BootstrapCommand(ctx context.Context, projectName string, verbose bool, provider client.Provider, cmd string, fabric client.FabricClient) error {
	if projectName == "" { // projectName is empty for "list --remote"
		term.Infof("Running CD command %q", cmd)
	} else {
		term.Infof("Running CD command %q in project %q", cmd, projectName)
	}
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	since := time.Now()
	etag, err := provider.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: projectName, Command: cmd})
	if err != nil || etag == "" {
		return err
	}

	if cmd == "down" || cmd == "destroy" {
		err = fabric.DeleteSubdomainZone(ctx, &defangv1.DeleteSubdomainZoneRequest{
			Project: projectName,
			Stack:   provider.GetStackNameForDomain(),
		})
		if err != nil {
			term.Warn("DeleteSubdomainZone failed:", err)
		} else {
			// If DeleteSubdomainZone succeeded, we're in the right workspace to mark the deployment as destroyed
			accountInfo, err := provider.AccountInfo(ctx)
			if err == nil {
				err = fabric.PutDeployment(ctx, &defangv1.PutDeploymentRequest{
					Deployment: &defangv1.Deployment{
						Action:            defangv1.DeploymentAction_DEPLOYMENT_ACTION_DOWN,
						Id:                etag,
						Project:           projectName,
						Provider:          accountInfo.Provider.Value(),
						ProviderAccountId: accountInfo.AccountID,
						ProviderString:    string(accountInfo.Provider),
						Region:            accountInfo.Region,
						Stack:             provider.GetStackName(),
						Timestamp:         timestamppb.New(time.Now()),
					},
				})
			}
			if err != nil {
				term.Debug("PutDeployment failed:", err)
				term.Warn("Unable to update deployment history, but deployment will proceed anyway.")
			}
		}
	}

	options := TailOptions{
		Deployment: etag,
		Since:      since,
		LogType:    logs.LogTypeBuild,
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

func BootstrapLocalList(ctx context.Context, provider client.Provider, allRegions bool) error {
	term.Debug("Running CD list")
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	stacks, err := provider.BootstrapList(ctx, allRegions)
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
