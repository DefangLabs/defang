package command

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var errFailedToReachStartedState = errors.New("failed to reach STARTED state")
var errDeploymentFailed = errors.New("deployment failed")

func waitServiceState(ctx context.Context, targetState defangv1.ServiceState, serviceList []string) error {
	// set up service status subscription (non-blocking)
	subscribeServiceStatusChan, err := cli.Subscribe(ctx, client, serviceList)
	if err != nil {
		term.Debugf("error subscribing to service status: %v", err)
		return err
	}

	serviceState := make(map[string]defangv1.ServiceState, len(serviceList))
	for _, name := range serviceList {
		serviceState[name] = defangv1.ServiceState_NOT_SPECIFIED
	}

	// monitor for when all services are completed to end this command
	for newStatus := range subscribeServiceStatusChan {
		term.Debugf("service %s with state ( %s ) and status: %s\n", newStatus.Name, newStatus.State, newStatus.Status)

		if _, ok := serviceState[newStatus.Name]; !ok {
			term.Debugf("unexpected service %s update", newStatus.Name)
			continue
		}

		// exit on detecting a FAILED state
		if newStatus.State == defangv1.ServiceState_SERVICE_FAILED {
			return errDeploymentFailed
		}

		serviceState[newStatus.Name] = newStatus.State

		if allInState(targetState, serviceState) {
			return nil // all services are in the target state
		}
	}

	term.Debug("service status subscription closed prematurely")
	return errFailedToReachStartedState
}

func allInState(targetState defangv1.ServiceState, serviceStates map[string]defangv1.ServiceState) bool {
	for _, state := range serviceStates {
		if state != targetState {
			return false
		}
	}
	return true
}
