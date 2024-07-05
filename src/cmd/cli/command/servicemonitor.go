package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func contextWithServiceStatus(ctx context.Context, targetStatus cli.ServiceStatus, serviceInfos []*defangv1.ServiceInfo) (context.Context, context.CancelFunc) {
	// New context to make sure tail is only cancelled when message about service status is printed
	newCtx, cancel := context.WithCancel(context.Background())
	go func() {
		err := waitServiceStatus(ctx, targetStatus, serviceInfos)
		if err == nil {
			cancel()
			return
		}

		if !errors.Is(err, context.Canceled) && !errors.Is(err, cli.ErrDryRun) && !errors.As(err, new(cliClient.ErrNotImplemented)) {
			term.Info("Service status monitoring failed, we will continue tailing the logs. Press Ctrl+C to detach.")
		}
		term.Debugf("failed to wait for service status: %v", err)

		<-ctx.Done() // Don't cancel tail until the original context is cancelled
		cancel()
	}()
	return newCtx, cancel

}
func waitServiceStatus(ctx context.Context, targetStatus cli.ServiceStatus, serviceInfos []*defangv1.ServiceInfo) error {
	serviceList := []string{}
	for _, serviceInfo := range serviceInfos {
		serviceList = append(serviceList, serviceInfo.Service.Name)
	}
	term.Debugf("Waiting for services %v to reach state: %s", serviceList, targetStatus)

	// set up service status subscription (non-blocking)
	serviceStatusUpdateStream, err := cli.Subscribe(ctx, client, serviceList)
	if err != nil {
		return fmt.Errorf("error subscribing to service status: %w", err)
	}
	defer serviceStatusUpdateStream.Close()

	serviceStatus := make(map[string]string, len(serviceList))
	for _, name := range serviceList {
		serviceStatus[name] = string(cli.ServiceUnknown)
	}

	// monitor for when all services are completed to end this command
	for {
		statusUpdate, err := serviceStatusUpdateStream.ServerStatus()
		if err != nil {
			return fmt.Errorf("service state monitoring terminated without all services reaching desired state %q: %w", targetStatus, err)
		}

		if _, ok := serviceStatus[statusUpdate.Name]; !ok {
			term.Debugf("unexpected service %s update", statusUpdate.Name)
			continue
		}
		serviceStatus[statusUpdate.Name] = statusUpdate.Status

		if allInStatus(targetStatus, serviceStatus) {
			for _, sInfo := range serviceInfos {
				sInfo.Status = string(targetStatus)
			}
			return nil
		}
	}
}

func allInStatus(targetStatus cli.ServiceStatus, serviceStatuses map[string]string) bool {
	for _, status := range serviceStatuses {
		if status != string(targetStatus) {
			return false
		}
	}
	return true
}
