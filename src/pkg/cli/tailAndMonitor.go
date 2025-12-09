package cli

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

func Monitor(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, deploymentID string) (ServiceStates, error) {
	if deploymentID == "" {
		panic("deploymentID must be a valid deployment ID")
	}
	if waitTimeout > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, waitTimeout)
		defer cancelTimeout()
	}

	svcStatusCtx, cancelSvcStatus := context.WithCancelCause(ctx)
	defer cancelSvcStatus(nil)

	_, computeServices := splitManagedAndUnmanagedServices(project.Services)

	for _, svc := range computeServices {
		term.Infof("%s [%s] %s\n", time.Now().Format(time.TimeOnly), svc, "PENDING")
	}

	var (
		serviceStates ServiceStates
		cdErr, svcErr error
	)
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		serviceStates, svcErr = WatchServiceState(svcStatusCtx, provider, project.Name, deploymentID, computeServices, func(msg *defangv1.SubscribeResponse, states *ServiceStates) error {
			// Print service status updates as they arrive
			for name, state := range *states {
				term.Infof("%s [%s] %s\n", time.Now().Format(time.TimeOnly), name, state.String())
			}
			return nil
		})
	}()

	go func() {
		defer wg.Done()
		if err := WaitForCdTaskExit(ctx, provider); err != nil {
			cdErr = err
			cancelSvcStatus(cdErr)
		}
	}()

	wg.Wait()
	pkg.SleepWithContext(ctx, 2*time.Second)

	return serviceStates, errors.Join(cdErr, svcErr)
}

func TailAndMonitor(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, tailOptions TailOptions) (ServiceStates, error) {
	tailOptions.Follow = true
	if tailOptions.Deployment == "" {
		panic("tailOptions.Deployment must be a valid deployment ID")
	}

	tailCtx, cancelTail := context.WithCancelCause(context.Background())
	defer cancelTail(nil) // to cancel tail and clean-up context

	errMonitoringDone := errors.New("monitoring done") // pseudo error to signal that monitoring is done

	var serviceStates ServiceStates
	var monitorErr error

	// Run Monitor in a goroutine
	go func() {
		serviceStates, monitorErr = Monitor(ctx, project, provider, waitTimeout, tailOptions.Deployment)
		pkg.SleepWithContext(ctx, 2*time.Second) // a delay before cancelling tail to make sure we get last status messages
		cancelTail(errMonitoringDone)            // cancel the tail when monitoring is done
	}()

	tailOptions.PrintBookends = false
	// blocking call to tail
	var tailErr error
	if err := Tail(tailCtx, provider, project.Name, tailOptions); err != nil {
		term.Debug("Tail stopped with", err, errors.Unwrap(err))

		if connect.CodeOf(err) == connect.CodePermissionDenied {
			term.Warn("Unable to tail logs. Waiting for the deployment to finish.")
			// If tail fails because of missing permission, we wait for the deployment to finish
			<-tailCtx.Done()
			// Get the actual error from the context so we won't print "Error: missing tail permission"
			err = context.Cause(tailCtx)
		}

		switch {
		case errors.Is(err, io.EOF):
			break // an end condition was detected; monitorErr might be nil

		case errors.Is(context.Cause(ctx), context.Canceled):
			term.Warn("Deployment is not finished. Service(s) might not be running.")

		case errors.Is(context.Cause(tailCtx), errMonitoringDone):
			break // the monitoring stopped the tail; monitorErr will have been set

		case errors.Is(context.Cause(ctx), context.DeadlineExceeded):
			// Tail was canceled when wait-timeout is reached; show a warning and exit with an error
			term.Warn("Wait-timeout exceeded, detaching from logs. Deployment still in progress.")
			fallthrough

		default:
			tailErr = err // report the error, in addition to the monitorErr
		}
	}

	return serviceStates, errors.Join(monitorErr, tailErr)
}

func CanMonitorService(service *compose.ServiceConfig) bool {
	// Services with "restart: no" are assumed to be one-off
	// tasks, so they are not monitored.
	if service.Restart == "no" {
		return false
	}

	if service.Extensions == nil {
		return true
	}

	return compose.IsComputeService(service)
}

func splitManagedAndUnmanagedServices(serviceInfos compose.Services) ([]string, []string) {
	var managedServices []string
	var unmanagedServices []string
	for _, service := range serviceInfos {
		if CanMonitorService(&service) {
			unmanagedServices = append(unmanagedServices, service.Name)
		} else {
			managedServices = append(managedServices, service.Name)
		}
	}

	return managedServices, unmanagedServices
}
