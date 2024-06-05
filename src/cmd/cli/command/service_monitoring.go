package command

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func monitorServiceStatus(ctx context.Context, targetStatus cli.ServiceStatus, serviceInfos []*defangv1.ServiceInfo, cancel context.CancelFunc) {
	serviceList := []string{}
	for _, serviceInfo := range serviceInfos {
		serviceList = append(serviceList, serviceInfo.Service.Name)
	}

	// set up service status subscription (non-blocking)
	serviceStatusChan, err := cli.Subscribe(ctx, client, serviceList)
	if err != nil {
		printEndpoints(serviceInfos)
		term.Debugf("error subscribing to service status: %v", err)
		return
	}

	// monitor for when all services are completed to end this command
	for serviceStatus := range serviceStatusChan {
		if isAllServicesAtSameStatus(targetStatus, *serviceStatus) {
			cancel()
			for _, sInfo := range serviceInfos {
				sInfo.Status = string(cli.ServiceStarted)
			}
			printEndpoints(serviceInfos)
			return
		}
	}

	// server channel closed without having all services completed
	term.Warnf("service state monitoring terminated without all services reaching desired state: %s", cli.ServiceStarted)
}

func isAllServicesAtSameStatus(targetStatus cli.ServiceStatus, serviceStatuses map[string]string) bool {
	allDone := true
	for _, status := range serviceStatuses {
		if status != string(targetStatus) {
			allDone = false
			break
		}
	}
	return allDone
}
