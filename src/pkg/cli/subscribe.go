package cli

import (
	"context"
	"errors"
	"iter"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var ErrNothingToMonitor = errors.New("no services to monitor")

type ServiceStates = map[string]defangv1.ServiceState

func WaitServiceState(
	ctx context.Context,
	provider client.Provider,
	targetState defangv1.ServiceState,
	projectName string,
	etag types.ETag,
	services []string,
) (ServiceStates, error) {
	term.Debugf("waiting for services %v to reach state %s\n", services, targetState) // TODO: don't print in Go-routine

	if len(services) == 0 {
		return nil, ErrNothingToMonitor
	}

	// Assume "services" are normalized service names
	subscribeRequest := defangv1.SubscribeRequest{Project: projectName, Etag: etag, Services: services}
	logs, err := provider.Subscribe(ctx, &subscribeRequest)
	if err != nil {
		return nil, err
	}

	next, stop := iter.Pull2(logs)
	defer stop()

	serviceStates := make(ServiceStates, len(services))
	// Make sure all services are in the map or `allInState` might return true too early
	for _, name := range services {
		serviceStates[name] = defangv1.ServiceState_NOT_SPECIFIED
	}

	// Monitor for when all services are completed to end this command
	for {
		msg, err, ok := next()
		if !ok {
			return serviceStates, nil
		}
		if err != nil {
			// Reconnect on transient errors (including ResourceExhausted — quota resets within
			// a minute and DelayBeforeRetry backs off exponentially up to 1 minute).
			if isTransientError(err) {
				if connect.CodeOf(err) == connect.CodeResourceExhausted {
					term.Warnf("quota exceeded; will retry subscribe stream after backoff: %v", err)
				} else {
					term.Debugf("WaitServiceState: transient error, reconnecting subscribe stream: %v", err)
				}
				if err := provider.DelayBeforeRetry(ctx); err != nil {
					return serviceStates, err
				}

				// A reconnected stream only observes state changes from here on; if the
				// target transition already happened while the previous stream was stalled,
				// no amount of reconnecting will ever see it again (#2241). Poll current
				// state directly as a fallback so a missed transition doesn't loop until
				// the caller's context deadline kills it.
				done, pollErr := pollServiceStates(ctx, provider, projectName, etag, targetState, serviceStates)
				if pollErr != nil {
					return serviceStates, pollErr
				}
				if done {
					return serviceStates, nil
				}

				stop() // stop the old iterator
				logs, err = provider.Subscribe(ctx, &subscribeRequest)
				if err != nil {
					return serviceStates, err
				}
				next, stop = iter.Pull2(logs)
				continue
			}
			return serviceStates, err
		}

		pendingServices := []string{}
		for _, service := range services {
			if serviceStates[service] != targetState {
				pendingServices = append(pendingServices, service)
			}
		}

		term.Infof("Waiting for services to finish deploying: %q\n", pendingServices) // TODO: don't print in Go-routine

		if msg == nil {
			continue
		}

		term.Debugf("Service update: %s: state=%s and status=%s\n", msg.Name, msg.State, msg.Status) // TODO: don't print in Go-routine

		if _, ok := serviceStates[msg.Name]; !ok {
			term.Debugf("unexpected service %s update", msg.Name) // TODO: don't print in Go-routine
			continue
		}
		if msg.State == defangv1.ServiceState_NOT_SPECIFIED {
			// We might get task/service states that do not map to a ServiceState; ignore those
			continue
		}

		if serviceStates[msg.Name] != targetState {
			serviceStates[msg.Name] = msg.State

			if err := failedStateError(msg.Name, msg.State, msg.Status); err != nil {
				return serviceStates, err
			}
		}

		if allInState(targetState, serviceStates) {
			return serviceStates, nil // all services are in the target state
		}
	}
}

// pollServiceStates fetches current service state directly via GetServices, independent of
// the log-tail stream, and merges any states for services we're tracking with a matching etag
// into serviceStates. It returns done=true once all tracked services have reached targetState.
// A failure to poll is not fatal: the caller falls back to reconnecting the log-tail stream.
func pollServiceStates(ctx context.Context, provider client.Provider, projectName string, etag types.ETag, targetState defangv1.ServiceState, serviceStates ServiceStates) (done bool, err error) {
	resp, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		term.Debugf("WaitServiceState: GetServices poll failed, continuing to wait for log stream: %v", err)
		return false, nil
	}

	for _, svc := range resp.Services {
		if svc.Service == nil || svc.Etag != etag || svc.State == defangv1.ServiceState_NOT_SPECIFIED {
			continue
		}
		name := svc.Service.Name
		if _, tracked := serviceStates[name]; !tracked {
			continue
		}
		serviceStates[name] = svc.State
		if err := failedStateError(name, svc.State, svc.Status); err != nil {
			return false, err
		}
	}

	return allInState(targetState, serviceStates), nil
}

func failedStateError(name string, state defangv1.ServiceState, status string) error {
	switch state {
	case defangv1.ServiceState_BUILD_FAILED, defangv1.ServiceState_DEPLOYMENT_FAILED:
		return client.ErrDeploymentFailed{Service: name, Message: status}
	}
	return nil
}

func allInState(targetState defangv1.ServiceState, serviceStates ServiceStates) bool {
	for _, state := range serviceStates {
		if state != targetState {
			return false
		}
	}
	return true
}
