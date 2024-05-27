package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func handleUserExit(cancel context.CancelFunc) {
	input := term.NewNonBlockingStdin()
	defer input.Close() // abort the read loop

	var b [1]byte
	for {
		if _, err := input.Read(b[:]); err != nil {
			return // exit goroutine
		}
		switch b[0] {
		case 3: // Ctrl-C
			cancel() // cancel the tail context
			return
		}
	}
}

func Subscribe(ctx context.Context, client client.Client, services []string) (<-chan *map[string]string, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	serviceStatus := make(map[string]string, len(services))
	noarmalizedServiceNameToServiceName := make(map[string]string, len(services))

	for i, service := range services {
		services[i] = NormalizeServiceName(service)
		noarmalizedServiceNameToServiceName[services[i]] = service
		serviceStatus[service] = "UNKNOWN"
	}

	serverStream, err := client.Subscribe(ctx, &defangv1.SubscribeRequest{Services: services})
	if err != nil {
		return nil, err
	}
	defer serverStream.Close()

	statusChan := make(chan *map[string]string, len(services))
	defer close(statusChan)

	if DoDryRun {
		statusChan <- &serviceStatus
		return statusChan, ErrDryRun
	}

	go func() {
		retryCount := 5
		for {

			// handle cancel from caller
			select {
			case <-ctx.Done():
				term.Debug("Context Done - exiting Subscribe goroutine")
				return
			default:
			}

			if retryCount == 0 {
				statusChan <- &serviceStatus
				term.Errorf("Subscribe failed - error on receive")
				return
			}

			if !serverStream.Receive() {
				term.Warnf("Subscribe Stream failed - will retry %d more times\n", retryCount)
				retryCount--
				continue
			}

			// reset the retry count after a successful receive
			retryCount = 5

			msg := serverStream.Msg()
			for _, servInfo := range msg.GetServices() {
				serviceName := noarmalizedServiceNameToServiceName[servInfo.Service.Name]
				serviceStatus[serviceName] = servInfo.Status
				statusChan <- &serviceStatus
				term.Debugf("Service(%s): %s with status %s\n", servInfo.Service, serviceName, servInfo.Status)
			}
		}
	}()

	return statusChan, nil
}
