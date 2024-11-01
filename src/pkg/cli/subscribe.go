package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrDeploymentFailed struct {
	Service string
}

func (e ErrDeploymentFailed) Error() string {
	return fmt.Sprintf("deployment failed for service %q", e.Service)
}

var ErrNothingToDeploy = errors.New("no services to deploy")

func WaitServiceState(ctx context.Context, provider client.Provider, targetState defangv1.ServiceState, etag string, services []string) error {
	term.Debugf("waiting for services %v to reach state %s\n", services, targetState) // TODO: don't print in Go-routine

	if DoDryRun {
		return ErrDryRun
	}

	if len(services) == 0 {
		return ErrNothingToDeploy
	}

	// Assume "services" are normalized service names
	subscribeRequest := defangv1.SubscribeRequest{Etag: etag, Services: services}
	serverStream, err := provider.Subscribe(ctx, &subscribeRequest)
	if err != nil {
		return err
	}

	serviceStates := make(map[string]defangv1.ServiceState, len(services))
	for _, name := range services {
		serviceStates[name] = defangv1.ServiceState_NOT_SPECIFIED
	}

	// Monitor for when all services are completed to end this command
	for {
		if !serverStream.Receive() {
			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if isTransientError(serverStream.Err()) {
				pkg.SleepWithContext(ctx, 1*time.Second)
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
			return ErrDeploymentFailed{msg.Name}
		}

		serviceStates[msg.Name] = msg.State

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
