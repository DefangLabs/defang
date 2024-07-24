package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type SubscribeServiceStatus struct {
	Name   string
	Status string
	State  defangv1.ServiceState
}

func Subscribe(ctx context.Context, client client.Client, etag string, services []string) (<-chan SubscribeServiceStatus, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("no services specified")
	}

	statusChan := make(chan SubscribeServiceStatus, len(services))
	if DoDryRun {
		defer close(statusChan)
		return statusChan, ErrDryRun
	}

	subscribeRequest := defangv1.SubscribeRequest{Etag: etag, Services: services}
	serverStream, err := client.Subscribe(ctx, &subscribeRequest)
	if err != nil {
		return nil, err
	}

	go func() {
		defer serverStream.Close()
		defer close(statusChan)
		for {
			if !serverStream.Receive() {
				// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
				if isTransientError(serverStream.Err()) {
					pkg.SleepWithContext(ctx, 1*time.Second)
					serverStream, err = client.Subscribe(ctx, &subscribeRequest)
					if err != nil {
						return
					}
					continue
				}
				return
			}

			msg := serverStream.Msg()
			if msg == nil {
				continue
			}

			subStatus := SubscribeServiceStatus{
				Name:   msg.Name,
				Status: msg.Status,
				State:  msg.State,
			}

			servInfo := msg.Service
			if subStatus.Name == "" && (servInfo != nil && servInfo.Service != nil) {
				subStatus.Name = servInfo.Service.Name
				subStatus.Status = servInfo.Status
				subStatus.State = convertServiceState(servInfo.Status)
			}

			statusChan <- subStatus
		}
	}()

	return statusChan, nil
}

// Deprecated: for old backend compatibility
func convertServiceState(status string) defangv1.ServiceState {
	switch strings.ToUpper(status) {
	default:
		return defangv1.ServiceState_NOT_SPECIFIED
	case "IN_PROGRESS", "STARTING":
		return defangv1.ServiceState_SERVICE_PENDING
	case "COMPLETED":
		return defangv1.ServiceState_SERVICE_COMPLETED
	case "FAILED":
		return defangv1.ServiceState_SERVICE_FAILED
	}
}
