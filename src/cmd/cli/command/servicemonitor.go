package command

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func waitServiceState(ctx context.Context, targetState defangv1.ServiceState, serviceInfos []*defangv1.ServiceInfo) error {
	serviceList := []string{}
	for _, serviceInfo := range serviceInfos {
		serviceList = append(serviceList, compose.NormalizeServiceName(serviceInfo.Service.Name))
	}

	// set up service status subscription (non-blocking)
	subscribeServiceStatusChan, err := cli.Subscribe(ctx, client, serviceList)
	if err != nil {
		term.Debugf("error subscribing to service status: %v", err)
		return err
	}

	serviceState := make(map[string]defangv1.ServiceState, len(serviceList))
	for _, name := range serviceList {
		serviceState[name] = defangv1.ServiceState_UNKNOWN
	}

	// monitor for when all services are completed to end this command
	for newStatus := range subscribeServiceStatusChan {
		if _, ok := serviceState[newStatus.Name]; !ok {
			term.Debugf("unexpected service %s update", newStatus.Name)
			continue
		}

		// exit on detecting a FAILED state
		if newStatus.State == defangv1.ServiceState_FAILED {
			return ErrDeploymentFailed
		}

		serviceState[newStatus.Name] = newStatus.State

		if allInState(targetState, serviceState) {
			for _, sInfo := range serviceInfos {
				sInfo.State = targetState
			}
			return nil
		}
	}

	return ErrFailedToReachStartedState
}

func allInState(targetState defangv1.ServiceState, serviceStates map[string]defangv1.ServiceState) bool {
	for _, state := range serviceStates {
		if state != targetState {
			return false
		}
	}
	return true
}
