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
	State  defangv1.ServiceState
}

func Subscribe(ctx context.Context, client client.Client, services []string) (<-chan SubscribeServiceStatus, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
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

			status := SubscribeServiceStatus{
				Name:   servInfo.Service.Name,
				Status: servInfo.Status,
				State:  servInfo.State,
			}

			statusChan <- status
			term.Debugf("service %s with state ( %s ) and status: %s\n", servInfo.Service.Name, servInfo.State.String(), servInfo.Status)
		}
	}()

	return statusChan, nil
}
