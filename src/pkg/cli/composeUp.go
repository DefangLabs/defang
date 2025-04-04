package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ComposeError struct {
	error
}

func (e ComposeError) Unwrap() error {
	return e.error
}

var errMonitoringDone = errors.New("monitoring done")

// ComposeUp validates a compose project and uploads the services using the client
func ComposeUp(ctx context.Context, project *compose.Project, c client.FabricClient, p client.Provider, upload compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *compose.Project, error) {
	if DoDryRun {
		upload = compose.UploadModeIgnore
	}

	// Validate the project configuration against the provider's configuration, but only if we are going to deploy.
	// FIXME: should not need to validate configs if we are doing preview, but preview will fail on missing configs.
	if upload != compose.UploadModeIgnore {
		listConfigNamesFunc := func(ctx context.Context) ([]string, error) {
			configs, err := p.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
			if err != nil {
				return nil, err
			}

			return configs.Names, nil
		}

		if err := compose.ValidateProjectConfig(ctx, project, listConfigNamesFunc); err != nil {
			return nil, project, &ComposeError{err}
		}
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, project, &ComposeError{err}
	}

	// Create a new project with only the necessary resources.
	// Do not modify the original project, because the caller needs it for debugging.
	fixedProject := project.WithoutUnnecessaryResources()

	if err := compose.FixupServices(ctx, p, fixedProject, upload); err != nil {
		return nil, project, err
	}

	bytes, err := fixedProject.MarshalYAML()
	if err != nil {
		return nil, project, err
	}

	if upload == compose.UploadModeIgnore {
		fmt.Println(string(bytes))
		return nil, project, ErrDryRun
	}

	delegateDomain, err := c.GetDelegateSubdomainZone(ctx)
	if err != nil {
		term.Debug("GetDelegateSubdomainZone failed:", err)
		return nil, project, errors.New("failed to get delegate domain")
	}

	deployRequest := &defangv1.DeployRequest{
		Mode:           mode,
		Project:        project.Name,
		Compose:        bytes,
		DelegateDomain: delegateDomain.Zone,
	}

	delegation, err := p.PrepareDomainDelegation(ctx, client.PrepareDomainDelegationRequest{
		DelegateDomain: delegateDomain.Zone,
		Preview:        upload == compose.UploadModePreview,
		Project:        project.Name,
	})
	if err != nil {
		return nil, project, err
	} else if delegation != nil {
		deployRequest.DelegationSetId = delegation.DelegationSetId
	}

	var resp *defangv1.DeployResponse
	if upload == compose.UploadModePreview {
		resp, err = p.Preview(ctx, deployRequest)
		if err != nil {
			return nil, project, err
		}
	} else {
		if delegation != nil && len(delegation.NameServers) > 0 {
			req := &defangv1.DelegateSubdomainZoneRequest{NameServerRecords: delegation.NameServers}
			_, err = c.DelegateSubdomainZone(ctx, req)
			if err != nil {
				return nil, project, err
			}
		}

		accountInfo, err := p.AccountInfo(ctx)
		if err != nil {
			return nil, project, err
		}

		timestamp := time.Now()
		resp, err = p.Deploy(ctx, deployRequest)
		if err != nil {
			return nil, project, err
		}

		err = c.PutDeployment(ctx, &defangv1.PutDeploymentRequest{
			Deployment: &defangv1.Deployment{
				Action:            defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP,
				Id:                resp.Etag,
				Project:           project.Name,
				Provider:          string(accountInfo.Provider()),
				ProviderAccountId: accountInfo.AccountID(),
				Timestamp:         timestamppb.New(timestamp),
			},
		})
		if err != nil {
			term.Debugf("PutDeployment failed: %v", err)
			term.Warn("Unable to update deployment history, but deployment will proceed anyway.")
		}
	}

	if term.DoDebug() {
		fmt.Println("Project:", project.Name)
		for _, serviceInfo := range resp.Services {
			PrintObject(serviceInfo.Service.Name, serviceInfo)
		}
	}
	return resp, project, nil
}

func TailUp(ctx context.Context, provider client.Provider, project *compose.Project, deploy *defangv1.DeployResponse, tailOptions TailOptions) error {
	if tailOptions.Deployment == "" {
		tailOptions.Deployment = deploy.Etag
	}
	if tailOptions.Since.IsZero() {
		tailOptions.Since = time.Now()
	}
	if tailOptions.LogType == logs.LogTypeUnspecified {
		tailOptions.LogType = logs.LogTypeAll
	}

	err := Tail(ctx, provider, project.Name, tailOptions)
	if err != nil {
		term.Debug("Tail stopped with", err)

		if connect.CodeOf(err) == connect.CodePermissionDenied {
			term.Warn("Unable to tail logs. Waiting for the deployment to finish.")
			// If tail fails because of missing permission, we wait for the deployment to finish
			<-ctx.Done()
			// Get the actual error from the context so we won't print "Error: missing tail permission"
			err = context.Cause(ctx)
		}

		if errors.Is(context.Cause(ctx), errMonitoringDone) {
			return nil // the monitoring stopped the tail, it will log reason
		}

		// The tail was canceled; check if it was because of deployment failure or explicit cancelation or wait-timeout reached
		if errors.Is(context.Cause(ctx), context.Canceled) {
			term.Warn("Deployment is not finished. Service(s) might not be running.")
			return err
		}

		if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			// Tail was canceled when wait-timeout is reached; show a warning and exit with an error
			term.Warn("Wait-timeout exceeded, detaching from logs. Deployment still in progress.")
			return err
		}

		return err
	}

	return nil
}

func WaitAndTail(ctx context.Context, project *compose.Project, client client.FabricClient, provider client.Provider, deploy *defangv1.DeployResponse, waitTimeout time.Duration, since time.Time, verbose bool) error {
	if DoDryRun {
		// If we are in dry-run mode, we don't need to wait for the deployment to complete
		return ErrDryRun
	}
	if waitTimeout >= 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, waitTimeout)
		defer cancelTimeout()
	}

	ctx, cancelTail := context.WithCancelCause(ctx)
	defer cancelTail(nil) // to cancel WaitServiceState and clean-up context
	svcStatusCtx, cancelSvcStatus := context.WithCancelCause(ctx)
	defer cancelSvcStatus(nil) // to cancel WaitServiceState and clean-up context

	const targetState = defangv1.ServiceState_DEPLOYMENT_COMPLETED
	_, unmanagedServices := SplitManagedAndUnmanagedServices(project.Services)

	var cdErr, svcErr error
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		// block on waiting for services to reach target state
		svcErr = WaitServiceState(svcStatusCtx, provider, targetState, project.Name, deploy.Etag, unmanagedServices)
	}()

	go func() {
		defer wg.Done()
		// block on waiting for cdTask to complete
		if err := WaitForCdTaskExit(ctx, provider); err != nil && !errors.Is(err, io.EOF) {
			cdErr = err
			// When CD fails, stop WaitServiceState
			cancelSvcStatus(cdErr)
		}
	}()

	go func() {
		wg.Wait()
		// a delay before cancelling tail to make sure we get last status messages
		time.Sleep(2 * time.Second)
		cancelTail(errMonitoringDone) // cancel the tail when both goroutines are done
	}()

	tailOptions := NewTailOptionsForDeploy(deploy, since, verbose)
	// blocking call to tail
	var tailErr error
	if err := TailUp(ctx, provider, project, deploy, tailOptions); err != nil && !errors.Is(err, context.Canceled) {
		tailErr = err
	}

	if err := errors.Join(cdErr, svcErr, tailErr); err != nil {
		return err
	}

	for _, service := range deploy.Services {
		service.State = targetState
	}

	return nil
}

func NewTailOptionsForDeploy(deploy *defangv1.DeployResponse, since time.Time, verbose bool) TailOptions {
	return TailOptions{
		Deployment: deploy.Etag,
		Since:      since,
		Raw:        false,
		Verbose:    verbose,
		LogType:    logs.LogTypeAll,
	}
}

func isManagedService(service compose.ServiceConfig) bool {
	if service.Extensions == nil {
		return false
	}

	return service.Extensions["x-defang-static-files"] != nil || service.Extensions["x-defang-redis"] != nil || service.Extensions["x-defang-postgres"] != nil
}

func SplitManagedAndUnmanagedServices(serviceInfos compose.Services) ([]string, []string) {
	var managedServices []string
	var unmanagedServices []string
	for _, service := range serviceInfos {
		if isManagedService(service) {
			managedServices = append(managedServices, service.Name)
		} else {
			unmanagedServices = append(unmanagedServices, service.Name)
		}
	}

	return managedServices, unmanagedServices
}
