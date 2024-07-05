package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ServiceStatusUpdate struct {
	Name   string
	Status string
}

type ServiceStatusStream struct {
	serverStream                       client.ServerStream[defangv1.SubscribeResponse]
	normalizedServiceNameToServiceName map[string]string
}

func (s *ServiceStatusStream) ServerStatus() (*ServiceStatusUpdate, error) {
	for s.serverStream.Receive() {
		msg := s.serverStream.Msg()
		if msg == nil {
			continue
		}

		servInfo := msg.GetService()
		if servInfo == nil || servInfo.Service == nil {
			continue
		}

		serviceName, ok := s.normalizedServiceNameToServiceName[servInfo.Service.Name]
		if !ok {
			term.Debugf("Unknown service %s in subscribe response\n", servInfo.Service.Name)
			continue
		}

		status := ServiceStatusUpdate{
			Name:   serviceName,
			Status: servInfo.Status,
		}

		term.Debugf("service %s with status %s\n", serviceName, servInfo.Status)
		return &status, nil
	}

	return nil, s.serverStream.Err()
}

func (s *ServiceStatusStream) Close() error {
	return s.serverStream.Close()
}

func Subscribe(ctx context.Context, client client.Client, services []string) (*ServiceStatusStream, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	if DoDryRun {
		return nil, ErrDryRun
	}

	normalizedServiceNameToServiceName := make(map[string]string, len(services))
	for i, service := range services {
		services[i] = compose.NormalizeServiceName(service)
		normalizedServiceNameToServiceName[services[i]] = service
	}

	serverStream, err := client.Subscribe(ctx, &defangv1.SubscribeRequest{Services: services})
	if err != nil {
		return nil, err
	}

	return &ServiceStatusStream{serverStream, normalizedServiceNameToServiceName}, nil
}
