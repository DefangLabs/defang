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

func Monitor(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, deploymentID string, watchCallback func(*defangv1.SubscribeResponse, *ServiceStates) error) (ServiceStates, error) {
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

	var (
		serviceStates ServiceStates
		cdErr, svcErr error
	)
	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		serviceStates, svcErr = WatchServiceState(svcStatusCtx, provider, project.Name, deploymentID, computeServices, watchCallback)
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
