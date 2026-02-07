package cli

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const targetServiceState = defangv1.ServiceState_DEPLOYMENT_COMPLETED

func Monitor(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, deploymentID string, watchCallback WatchCallback) (ServiceStates, error) {
	if deploymentID == "" {
		panic("deploymendID must be provided to monitor deployment")
	}
	if waitTimeout > 0 {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, waitTimeout)
		defer cancelTimeout()
	}

	svcStatusCtx, cancelSvcStatus := context.WithCancelCause(ctx)
	defer cancelSvcStatus(nil) // to cancel WaitServiceState and clean-up context

	_, computeServices := splitManagedAndUnmanagedServices(project.Services)

	var serviceStates ServiceStates
	var cdErr, svcErr error

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		serviceStates, svcErr = WatchServiceState(svcStatusCtx, provider, targetServiceState, project.Name, deploymentID, computeServices, watchCallback)
	}()

	go func() {
		defer wg.Done()
		// block on waiting for cdTask to complete
		if err := WaitForCdTaskExit(ctx, provider); err != nil {
			cdErr = err
			// When CD fails, stop WaitServiceState
			cancelSvcStatus(cdErr)
		}
	}()

	wg.Wait()
	pkg.SleepWithContext(ctx, 2*time.Second)

	return serviceStates, errors.Join(cdErr, svcErr)
}
