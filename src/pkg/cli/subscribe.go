package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Subscribe(ctx context.Context, client client.Client, services []string) (<-chan *map[string]string, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	serviceStatus := make(map[string]string, len(services))
	noarmalizedServiceNameToServiceName := make(map[string]string, len(services))

	for i, service := range services {
		services[i] = NormalizeServiceName(service)
		noarmalizedServiceNameToServiceName[services[i]] = service
		serviceStatus[service] = string(types.ServiceUnknown)
	}

	serverStream, err := client.Subscribe(ctx, &defangv1.SubscribeRequest{Services: services})
	if err != nil {
		return nil, err
	}
	statusChan := make(chan *map[string]string, len(services))
	if DoDryRun {
		defer close(statusChan)
		statusChan <- &serviceStatus
		return statusChan, ErrDryRun
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
				term.Warn("Subscribe Stream failed - stopping\n")
				return
			}

			msg := serverStream.Msg()
			for _, servInfo := range msg.GetServices() {
				serviceName := noarmalizedServiceNameToServiceName[servInfo.Service.Name]
				serviceStatus[serviceName] = servInfo.Status
				statusChan <- &serviceStatus
				term.Debugf("service %s with status %s\n", serviceName, servInfo.Status)
			}
		}
	}()

	return statusChan, nil
}
