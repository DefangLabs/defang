package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var ErrNothingToMonitor = errors.New("no services to monitor")

func WaitServiceState(
	ctx context.Context,
	provider client.Provider,
	targetState defangv1.ServiceState,
	projectName string,
	etag types.ETag,
	services []string,
) error {
	term.Debugf("waiting for services %v to reach state %s\n", services, targetState) // TODO: don't print in Go-routine

	if len(services) == 0 {
		return ErrNothingToMonitor
	}

	// Assume "services" are normalized service names
	subscribeRequest := defangv1.SubscribeRequest{Project: projectName, Etag: etag, Services: services}
	serverStream, err := provider.Subscribe(ctx, &subscribeRequest)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // to ensure we close the stream and clean-up this context

	go func() {
		<-ctx.Done()
		serverStream.Close()
	}()

	serviceStates := make(map[string]defangv1.ServiceState, len(services))
	for _, name := range services {
		serviceStates[name] = defangv1.ServiceState_NOT_SPECIFIED
	}

	// Monitor for when all services are completed to end this command
	for {
		if !serverStream.Receive() {
			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if isTransientError(serverStream.Err()) {
				if err := provider.DelayBeforeRetry(ctx); err != nil {
					return err
				}
				serverStream, err = provider.Subscribe(ctx, &subscribeRequest)
				if err != nil {
					return err
				}
				continue
			}
			return serverStream.Err()
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

		// exit early on detecting a FAILED state
		switch msg.State {
		case defangv1.ServiceState_BUILD_FAILED, defangv1.ServiceState_DEPLOYMENT_FAILED:
			return pkg.ErrDeploymentFailed{Service: msg.Name, Message: msg.Status}
		}
		serviceStates[msg.Name] = msg.State

		if !allServicesUpdated(serviceStates) {
			continue
		}
		if allInState(targetState, serviceStates) {
			return nil // all services are in the target state
		}
	}
}

func allInState(targetState defangv1.ServiceState, serviceStates map[string]defangv1.ServiceState) bool {
	for _, state := range serviceStates {
		if state != targetState {
			return false
		}
	}
	return true
}

func allServicesUpdated(serviceStates map[string]defangv1.ServiceState) bool {
	for _, state := range serviceStates {
		if state == defangv1.ServiceState_NOT_SPECIFIED {
			return false
		}
	}
	return true
}
