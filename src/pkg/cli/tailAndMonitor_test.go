package cli

import (
	"context"
	"io"
	"iter"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/require"
)

type mockSubscribeData struct {
	index int
	resps []*defangv1.SubscribeResponse
	err   error
}

type mockTailAndMonitorProvider struct {
	client.MockProvider
	getDeploymentStatusErr error
	subs                   map[types.ETag]*mockSubscribeData
}

func (m *mockTailAndMonitorProvider) GetDeploymentStatus(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, m.getDeploymentStatusErr
}

func (m *mockTailAndMonitorProvider) Subscribe(_ context.Context, req *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	data := m.subs[req.Etag]
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		for data.index < len(data.resps) {
			resp := data.resps[data.index]
			data.index++
			if resp == nil {
				if data.err != nil {
					yield(nil, data.err)
				}
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}, nil
}

func (m *mockTailAndMonitorProvider) QueryLogs(ctx context.Context, r *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	return client.ServerStreamIterCtx(ctx, client.NewMockWaitStream[defangv1.TailResponse]()), ctx.Err()
}

func (m *mockTailAndMonitorProvider) DelayBeforeRetry(ctx context.Context) error {
	return ctx.Err()
}

func TestTailAndMonitor(t *testing.T) {
	mockProvider := &mockTailAndMonitorProvider{
		getDeploymentStatusErr: io.EOF, //client.ErrDeploymentFailed{}, // done
		subs: map[types.ETag]*mockSubscribeData{
			"deployment12": {
				err: io.ErrUnexpectedEOF, // reconnection
				resps: []*defangv1.SubscribeResponse{
					nil, // reconnect
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_DEPLOYMENT_PENDING},
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_DEPLOYMENT_PENDING},
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                    // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                    // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "api", Status: " : 5d5a308a19fd48f3972ae9aa74768f29", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_PROVISIONING
					{Service: nil, Name: "web", Status: " : 346b2dbd236b4a24ab86abcfafda4eef", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_PROVISIONING
					{Service: nil, Name: "web", Status: " : 346b2dbd236b4a24ab86abcfafda4eef", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_PENDING
					{Service: nil, Name: "api", Status: " : 5d5a308a19fd48f3972ae9aa74768f29", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_PENDING
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                 // SERVICE_STEADY_STATE
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                 // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "web", Status: " : 346b2dbd236b4a24ab86abcfafda4eef", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_ACTIVATING
					{Service: nil, Name: "api", Status: " : 5d5a308a19fd48f3972ae9aa74768f29", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_ACTIVATING
					{Service: nil, Name: "web", Status: " : 346b2dbd236b4a24ab86abcfafda4eef", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_RUNNING
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                    // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                    // SERVICE_STEADY_STATE
					{Service: nil, Name: "api", Status: " : 5d5a308a19fd48f3972ae9aa74768f29", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_RUNNING
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                    // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},                                    // SERVICE_STEADY_STATE
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_DEPLOYMENT_PENDING},
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED}, // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "auth", Status: " : 0b54ed2ba5fa4ec7bbc2abd658c5684c", State: defangv1.ServiceState_NOT_SPECIFIED},
					{Service: nil, Name: "auth", Status: " : 0b54ed2ba5fa4ec7bbc2abd658c5684c", State: defangv1.ServiceState_NOT_SPECIFIED},
					{Service: nil, Name: "auth", Status: " : 0b54ed2ba5fa4ec7bbc2abd658c5684c", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_ACTIVATING
					{Service: nil, Name: "auth", Status: " : 0b54ed2ba5fa4ec7bbc2abd658c5684c", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_RUNNING
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_DEPLOYMENT_PENDING},
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED}, // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},   // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},   // SERVICE_STEADY_STATE
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},    // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "web", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},    // SERVICE_STEADY_STATE
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED}, // SERVICE_STEADY_STATE
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED}, // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "hasura", Status: " : c7ed06d1bd824a97a6a2b1435f20511b", State: defangv1.ServiceState_NOT_SPECIFIED},
					{Service: nil, Name: "hasura", Status: " : c7ed06d1bd824a97a6a2b1435f20511b", State: defangv1.ServiceState_NOT_SPECIFIED},
					{Service: nil, Name: "hasura", Status: " : c7ed06d1bd824a97a6a2b1435f20511b", State: defangv1.ServiceState_NOT_SPECIFIED},
					{Service: nil, Name: "hasura", Status: " : c7ed06d1bd824a97a6a2b1435f20511b", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_ACTIVATING
					{Service: nil, Name: "hasura", Status: " : c7ed06d1bd824a97a6a2b1435f20511b", State: defangv1.ServiceState_NOT_SPECIFIED}, // TASK_RUNNING
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED}, // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "hasura", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED}, // SERVICE_STEADY_STATE
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},    // SERVICE_STEADY_STATE
					{Service: nil, Name: "api", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},    // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},   // CAPACITY_PROVIDER_STEADY_STATE
					{Service: nil, Name: "auth", Status: "", State: defangv1.ServiceState_NOT_SPECIFIED},   // SERVICE_STEADY_STATE
				},
			},
		},
	}
	project := &compose.Project{
		Services: compose.Services{
			"hasura-postgres": compose.ServiceConfig{
				Name: "hasura-postgres",
				Extensions: map[string]any{
					"x-defang-postgres": true,
				},
			},
			"auth-redis": compose.ServiceConfig{
				Name: "auth-redis",
				Extensions: map[string]any{
					"x-defang-redis": true,
				},
			},
			"auth":   compose.ServiceConfig{Name: "auth"},
			"web":    compose.ServiceConfig{Name: "web"},
			"hasura": compose.ServiceConfig{Name: "hasura"},
			"api":    compose.ServiceConfig{Name: "api"},
		},
	}
	states, err := TailAndMonitor(t.Context(), project, mockProvider, time.Minute, TailOptions{
		Deployment: "deployment12",
	})
	require.NoError(t, err)
	require.Equal(t, ServiceStates{
		"web":    defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		"api":    defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		"auth":   defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		"hasura": defangv1.ServiceState_DEPLOYMENT_COMPLETED,
	}, states)
}
