package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type SubscribeServiceStatus struct {
	Name   string
	Status string
}

func Subscribe(ctx context.Context, client client.Client, services []string) (<-chan SubscribeServiceStatus, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	normalizedServiceNameToServiceName := make(map[string]string, len(services))

	for i, service := range services {
		services[i] = NormalizeServiceName(service)
		normalizedServiceNameToServiceName[services[i]] = service
	}

	statusChan := make(chan SubscribeServiceStatus, len(services))
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
				term.Debug("Subscribe Stream closed", serverStream.Err())
				return
			}

			msg := serverStream.Msg()
			if msg == nil {
				continue
			}

			servInfo := msg.GetService()
			if servInfo == nil || servInfo.Service == nil {
				continue
			}

			serviceName, ok := normalizedServiceNameToServiceName[servInfo.Service.Name]
			if !ok {
				term.Debugf("Unknown service %s in subscribe response\n", servInfo.Service.Name)
				continue
			}
			status := SubscribeServiceStatus{
				Name:   serviceName,
				Status: servInfo.Status,
			}

			statusChan <- status
			term.Debugf("service %s with status %s\n", serviceName, servInfo.Status)
		}
	}()

	return statusChan, nil
}
