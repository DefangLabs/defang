package cli

import (
	"context"
	"errors"
	"iter"

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
			// Reconnect on transient errors
			if isTransientError(err) {
				if err := provider.DelayBeforeRetry(ctx); err != nil {
					return serviceStates, err
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

		if msg == nil {
			continue
		}

		term.Debugf("service %s with state ( %s ) and status: %s\n", msg.Name, msg.State, msg.Status) // TODO: don't print in Go-routine

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

			// exit early on detecting a FAILED state
			switch msg.State {
			case defangv1.ServiceState_BUILD_FAILED, defangv1.ServiceState_DEPLOYMENT_FAILED:
				return serviceStates, client.ErrDeploymentFailed{Service: msg.Name, Message: msg.Status}
			}
		}

		if allInState(targetState, serviceStates) {
			return serviceStates, nil // all services are in the target state
		}
	}
}

func allInState(targetState defangv1.ServiceState, serviceStates ServiceStates) bool {
	for _, state := range serviceStates {
		if state != targetState {
			return false
		}
	}
	return true
}
