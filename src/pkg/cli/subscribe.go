package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Subscribe(ctx context.Context, client client.Client, services []string) (<-chan *map[string]string, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	serviceStatus := make(map[string]string, len(services))
	normalizedServiceNameToServiceName := make(map[string]string, len(services))

	for i, service := range services {
		services[i] = NormalizeServiceName(service)
		normalizedServiceNameToServiceName[services[i]] = service
		serviceStatus[service] = string(ServiceUnknown)
	}

	statusChan := make(chan *map[string]string, len(services))
	if DoDryRun {
		defer close(statusChan)
		return statusChan, ErrDryRun
	}

	serverStream, err := client.Subscribe(ctx, &defangv1.SubscribeRequest{Services: services})
	if err != nil {
		return nil, err
	}

	go func() {
		defer serverStream.Close()
		defer close(statusChan)
		for {

			// handle cancel from caller
			select {
			case <-ctx.Done():
				term.Debug("Context Done - exiting Subscribe goroutine")
				return
			default:
			}

			if !serverStream.Receive() {
				term.Debug("Subscribe Stream closed")
				return
			}

			msg := serverStream.Msg()
			for _, servInfo := range msg.GetServices() {
				serviceName, ok := normalizedServiceNameToServiceName[servInfo.Service.Name]
				if !ok {
					term.Warnf("Unknown service %s in subscribe response\n", servInfo.Service.Name)
					continue
				}
				serviceStatus[serviceName] = servInfo.Status
				statusChan <- &serviceStatus
				term.Debugf("service %s with status %s\n", serviceName, servInfo.Status)
			}
		}
	}()

	return statusChan, nil
}