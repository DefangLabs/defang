package command

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func waitServiceStatus(ctx context.Context, targetStatus cli.ServiceStatus, serviceInfos []*defangv1.ServiceInfo) error {
	serviceList := []string{}
	for _, serviceInfo := range serviceInfos {
		serviceList = append(serviceList, serviceInfo.Service.Name)
	}

	// set up service status subscription (non-blocking)
	subscribeServiceStatusChan, err := cli.Subscribe(ctx, client, serviceList)
	if err != nil {
		term.Debugf("error subscribing to service status: %v", err)
		return err
	}

	serviceStatus := make(map[string]string, len(serviceList))
	for _, name := range serviceList {
		serviceStatus[name] = string(cli.ServiceUnknown)
	}

	// monitor for when all services are completed to end this command
	for newStatus := range subscribeServiceStatusChan {
		if _, ok := serviceStatus[newStatus.Name]; !ok {
			term.Debugf("unexpected service %s update", newStatus.Name)
			continue
		}

		serviceStatus[newStatus.Name] = newStatus.Status

		if allInStatus(targetStatus, serviceStatus) {
			for _, sInfo := range serviceInfos {
				sInfo.Status = string(targetStatus)
			}
			return nil
		}
	}

	return fmt.Errorf("service state monitoring terminated without all services reaching desired state: %s", targetStatus)
}

func allInStatus(targetStatus cli.ServiceStatus, serviceStatuses map[string]string) bool {
	for _, status := range serviceStatuses {
		if status != string(targetStatus) {
			return false
		}
	}
	return true
}
