package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var ErrNothingToMonitor = errors.New("no services to monitor")

type ServiceStates = map[string]defangv1.ServiceState

func WatchServiceState(
	ctx context.Context,
	provider client.Provider,
	projectName string,
	etag types.ETag,
	services []string,
	cb func(*defangv1.SubscribeResponse, *ServiceStates) error,
) (ServiceStates, error) {
	if len(services) == 0 {
		return nil, ErrNothingToMonitor
	}

	// Assume "services" are normalized service names
	subscribeRequest := defangv1.SubscribeRequest{Project: projectName, Etag: etag, Services: services}
	serverStream, err := provider.Subscribe(ctx, &subscribeRequest)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // to ensure we close the stream and clean-up this context

	go func() {
		<-ctx.Done()
		serverStream.Close()
	}()

	serviceStates := make(ServiceStates, len(services))
	// Make sure all services are in the map or `allInState` might return true too early
	for _, name := range services {
		serviceStates[name] = defangv1.ServiceState_NOT_SPECIFIED
	}

	// Monitor for when all services are completed to end this command
	for {
		if !serverStream.Receive() {
			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if isTransientError(serverStream.Err()) {
				if err := provider.DelayBeforeRetry(ctx); err != nil {
					return serviceStates, err
				}
				serverStream, err = provider.Subscribe(ctx, &subscribeRequest)
				if err != nil {
					return serviceStates, err
				}
				continue
			}
			return serviceStates, serverStream.Err()
		}

		msg := serverStream.Msg()
		if msg == nil {
			continue
		}

		term.Debugf("service %s with state ( %s ) and status: %s\n", msg.Name, msg.State, msg.Status) // TODO: don't print in Go-routine

		if _, ok := serviceStates[msg.Name]; !ok {
			term.Debugf("unexpected service %s update", msg.Name) // TODO: don't print in Go-routine
			continue
		}

		serviceStates[msg.Name] = msg.State
		err := cb(msg, &serviceStates)
		if err != nil {
			if errors.Is(err, client.ErrDeploymentSucceeded) {
				return serviceStates, nil
			}
			return serviceStates, err
		}
	}
}

func WaitServiceState(
	ctx context.Context,
	provider client.Provider,
	targetState defangv1.ServiceState,
	projectName string,
	etag types.ETag,
	services []string,
) (ServiceStates, error) {
	term.Debugf("waiting for services %v to reach state %s\n", services, targetState) // TODO: don't print in Go-routine

	return WatchServiceState(ctx, provider, projectName, etag, services, func(msg *defangv1.SubscribeResponse, serviceStates *ServiceStates) error {
		// exit early on detecting a FAILED state
		switch msg.State {
		case defangv1.ServiceState_BUILD_FAILED, defangv1.ServiceState_DEPLOYMENT_FAILED:
			return client.ErrDeploymentFailed{Service: msg.Name, Message: msg.Status}
		}

		if allInState(targetState, *serviceStates) {
			return client.ErrDeploymentSucceeded // signal successful completion
		}

		return nil
	})
}

func allInState(targetState defangv1.ServiceState, serviceStates ServiceStates) bool {
	for _, state := range serviceStates {
		if state != targetState {
			return false
		}
	}
	return true
}
