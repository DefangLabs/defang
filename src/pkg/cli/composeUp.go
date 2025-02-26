package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
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
			term.Debug("PutDeployment failed:", err)
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
	if tailOptions.Etag == "" {
		tailOptions.Etag = deploy.Etag
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

		// The tail was canceled; check if it was because of deployment failure or explicit cancelation or wait-timeout reached
		if errors.Is(context.Cause(ctx), context.Canceled) {
			// Tail was canceled by the user before deployment completion/failure; show a warning and exit with an error
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
	ctx, cancelTail := context.WithCancelCause(ctx)
	defer cancelTail(nil) // to cancel WaitServiceState and clean-up context

	if waitTimeout >= 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, waitTimeout)
		defer cancelTimeout()
	}

	errCompleted := errors.New("deployment succeeded") // tail canceled because of deployment completion

	const targetState = defangv1.ServiceState_DEPLOYMENT_COMPLETED
	_, unmanagedServices := SplitManagedAndUnmanagedServices(project.Services)
	go func() {
		if err := WaitServiceState(ctx, provider, targetState, project.Name, deploy.Etag, unmanagedServices); err != nil {
			var errDeploymentFailed pkg.ErrDeploymentFailed
			if errors.As(err, &errDeploymentFailed) {
				cancelTail(err)
			} else if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				term.Warnf("error waiting for deployment completion: %v", err) // TODO: don't print in Go-routine
			}
		} else {
			for _, service := range deploy.Services {
				service.State = targetState
			}

			cancelTail(errCompleted)
		}
	}()

	tailOptions := NewTailOptionsForDeploy(deploy, since, verbose)
	// blocking call to tail
	err := TailUp(ctx, provider, project, deploy, tailOptions)
	var errDeploymentFailed pkg.ErrDeploymentFailed
	if errors.As(context.Cause(ctx), &errDeploymentFailed) {
		return errDeploymentFailed
	}
	if !errors.Is(context.Cause(ctx), errCompleted) {
		return err
	}

	return nil
}

func NewTailOptionsForDeploy(deploy *defangv1.DeployResponse, since time.Time, verbose bool) TailOptions {
	return TailOptions{
		Etag:    deploy.Etag,
		Since:   since,
		Raw:     false,
		Verbose: verbose,
		LogType: logs.LogTypeAll,
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
