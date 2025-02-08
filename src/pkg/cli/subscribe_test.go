package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// MockSubscribeServerStream mocks the stream response for Subscribe.
type MockSubscribeServerStream struct {
	index int
	Resps []*defangv1.SubscribeResponse
	Error error
}

func (*MockSubscribeServerStream) Close() error {
	return nil
}

func (m *MockSubscribeServerStream) Receive() bool {
	if m.index >= len(m.Resps) {
		return false
	}
	m.index++
	return true
}

func (m *MockSubscribeServerStream) Msg() *defangv1.SubscribeResponse {
	if m.index == 0 || m.index > len(m.Resps) {
		return nil
	}
	return m.Resps[m.index-1]
}

func (m *MockSubscribeServerStream) Err() error {
	return m.Error
}

// mockSubscribeProvider mocks the provider for Subscribe.
type mockSubscribeProvider struct {
	client.MockProvider
	Reqs []*defangv1.SubscribeRequest
}

func (m *mockSubscribeProvider) Subscribe(
	_ context.Context,
	req *defangv1.SubscribeRequest,
) (client.ServerStream[defangv1.SubscribeResponse], error) {
	m.Reqs = append(m.Reqs, req)

	responses := map[string][]*defangv1.SubscribeResponse{
		"etag1": {
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
		},
		"etag2": {
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
			{
				Name:  "service2",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service2",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service2",
				State: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
		},
		"etag3": {
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_FAILED,
			},
		},
		"etag4": {
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_DEPLOYMENT_FAILED,
			},
		},
		"etag5": {
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service1",
				State: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
			{
				Name:  "service2",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service2",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service2",
				State: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
			{
				Name:  "service3",
				State: defangv1.ServiceState_BUILD_QUEUED,
			},
			{
				Name:  "service3",
				State: defangv1.ServiceState_BUILD_PROVISIONING,
			},
			{
				Name:  "service3",
				State: defangv1.ServiceState_DEPLOYMENT_FAILED,
			},
		},
	}[req.Etag]
	return &MockSubscribeServerStream{Resps: responses}, nil
}

func TestWaitServiceState(t *testing.T) {
	ctx := context.Background()
	provider := &mockSubscribeProvider{}

	pass_tests := []struct {
		etag        string
		services    []string
		targetState defangv1.ServiceState
	}{
		{
			etag:        "etag1",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		},
		{
			etag:        "etag2",
			services:    []string{"service1", "service2"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		},
	}

	fail_tests := []struct {
		etag        string
		services    []string
		targetState defangv1.ServiceState
	}{
		{
			etag:        "etag3",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		},
	}

	for _, tt := range pass_tests {
		t.Run("pass", func(t *testing.T) {
			err := WaitServiceState(ctx, provider, tt.targetState, tt.etag, tt.services)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}

	for _, tt := range fail_tests {
		t.Run("fail", func(t *testing.T) {
			err := WaitServiceState(ctx, provider, tt.targetState, tt.etag, tt.services)
			if err == nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !errors.As(err, &pkg.ErrDeploymentFailed{}) {
				t.Errorf("Expected ErrDeploymentFailed but got %v", err)
			}
		})
	}

	// Validate that Subscribe was called with expected parameters
	if len(provider.Reqs) == 0 {
		t.Errorf("Expected Subscribe to be called but got 0 requests")
	}
}
