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

	stream := &MockSubscribeServerStream{
		Resps: []*defangv1.SubscribeResponse{
			{Name: "service1", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
			{Name: "service2", State: defangv1.ServiceState_BUILD_FAILED},
			{Name: "service3", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
		},
	}
	return stream, nil
}

func TestWaitServiceState(t *testing.T) {
	ctx := context.Background()
	const targetState = defangv1.ServiceState_DEPLOYMENT_COMPLETED
	provider := &mockSubscribeProvider{}

	tests := []struct {
		name      string
		services  []string
		expectErr bool
	}{
		{
			name:      "state with DEPLOYMENT_COMPLETED",
			services:  []string{"service1", "service3"},
			expectErr: false,
		},
		{
			name:      "state with BUILD_FAILED",
			services:  []string{"service2"},
			expectErr: true,
		},
		{
			name:      "state with DEPLOYMENT_COMPLETED and BUILD_FAILED",
			services:  []string{"service1", "service2", "service3"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WaitServiceState(ctx, provider, targetState, "EtagSomething", tt.services)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if !errors.As(err, &pkg.ErrDeploymentFailed{}) {
					t.Errorf("Expected ErrDeploymentFailed but got %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}

	// Validate that Subscribe was called with expected parameters
	if len(provider.Reqs) == 0 {
		t.Errorf("Expected Subscribe to be called but got 0 requests")
	}
}
