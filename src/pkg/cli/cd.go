package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
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

	var statesUrl, eventsUrl string
	if _, ok := provider.(*client.PlaygroundProvider); !ok { // Do not need upload URLs for Playground
		var err error
		statesUrl, eventsUrl, err = GetStatesAndEventsUploadUrls(ctx, projectName, provider, fabric)
		if err != nil {
			return "", err
		}
	}

	etag, err := provider.CdCommand(ctx, client.CdCommandRequest{Project: projectName, Command: command, StatesUrl: statesUrl, EventsUrl: eventsUrl})
	if err != nil || etag == "" {
		return "", err
	}

	action := defangv1.DeploymentAction_DEPLOYMENT_ACTION_REFRESH
	switch command {
	case client.CdCommandDown, client.CdCommandDestroy:
		err := deleteSubdomain(ctx, projectName, provider, fabric)
		if err != nil {
			term.Warn("Unable to update deployment history; deployment will proceed anyway.")
			break
		}
		// Update deployment table to mark deployment as destroyed only after successful deletion of the subdomain
		action = defangv1.DeploymentAction_DEPLOYMENT_ACTION_DOWN
		fallthrough
	case client.CdCommandRefresh:
		err := putDeploymentAndStack(ctx, provider, fabric, nil, putDeploymentParams{
			Action:      action,
			ETag:        etag,
			ProjectName: projectName,
			StatesUrl:   statesUrl,
			EventsUrl:   eventsUrl,
		})
		if err != nil {
			term.Debug("Failed to record deployment:", err)
			term.Warn("Unable to update deployment history; deployment will proceed anyway.")
		}
	}
	return etag, nil
}

func deleteSubdomain(ctx context.Context, projectName string, provider client.Provider, fabric client.FabricClient) error {
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
	pulumiStack, _, _ := strings.Cut(name, " ") // in case name contains extra info like "proj/stack {workspace} [region]"
	projectName, stackName, _ = strings.Cut(pulumiStack, "/")
	return projectName, stackName
}

func CdListFromStorage(ctx context.Context, provider client.Provider, allRegions bool) error {
	term.Debug("Running CD list")
	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	stacks, err := provider.CdList(ctx, allRegions)
	if err != nil {
		return err
	}

	var count int
	for stackInfo := range stacks {
		count++
		if !allRegions {
			stackInfo, _, _ = strings.Cut(stackInfo, " ") // remove extra info like "{workspace} [region]"
		}
		term.Println(" -", stackInfo) // TODO: json output mode
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

func GetStatesAndEventsUploadUrls(ctx context.Context, projectName string, provider client.Provider, fabric client.FabricClient) (statesUrl string, eventsUrl string, err error) {
	// Allow overriding upload URLs via environment variables
	statesUrl, eventsUrl = os.Getenv("DEFANG_STATES_UPLOAD_URL"), os.Getenv("DEFANG_EVENTS_UPLOAD_URL")
	suffix := pkg.RandomID()

	if statesUrl == "" {
		statesResp, err := fabric.CreateUploadURL(ctx, &defangv1.UploadURLRequest{
			Project:  projectName,
			Stack:    provider.GetStackName(),
			Filename: fmt.Sprintf("states-%v.json", suffix),
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to create states upload URL: %w", err)
		}
		statesUrl = statesResp.Url
	}
	if eventsUrl == "" {
		eventsResp, err := fabric.CreateUploadURL(ctx, &defangv1.UploadURLRequest{
			Project:  projectName,
			Stack:    provider.GetStackName(),
			Filename: fmt.Sprintf("events-%v.json", suffix),
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to create events upload URL: %w", err)
		}
		eventsUrl = eventsResp.Url
	}
	return statesUrl, eventsUrl, nil
}
