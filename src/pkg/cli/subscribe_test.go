package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
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

	resps, ok := map[string][]*defangv1.SubscribeResponse{
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

	if !ok {
		panic("unexpected etag")
	}

	stream := &MockSubscribeServerStream{Resps: resps}
	return stream, nil
}

func TestWaitServiceState(t *testing.T) {
	ctx := context.Background()
	provider := &mockSubscribeProvider{}

	noErrTests := []struct {
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

	errTests := []struct {
		etag        string
		services    []string
		targetState defangv1.ServiceState
	}{
		{
			etag:        "etag3",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		},
		{
			etag:        "etag4",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
		},
		{
			etag:        "etag5",
			services:    []string{"service1", "service2", "service3"},
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

	for _, tt := range errTests {
		t.Run("Expect Error", func(t *testing.T) {
			err := WaitServiceState(ctx, provider, tt.targetState, tt.etag, tt.services)
			if err == nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !errors.As(err, &pkg.ErrDeploymentFailed{}) {
				t.Errorf("Expected ErrDeploymentFailed but got %v", err)
			}
		})
	}

	if len(provider.Reqs) == 0 {
		t.Errorf("Expected Subscribe to be called but got 0 requests")
	}
}

type MockSubscribeServerStreamForReconnectTest struct {
	Error error
	retry int
}

func (*MockSubscribeServerStreamForReconnectTest) Close() error {
	return nil
}

func (m *MockSubscribeServerStreamForReconnectTest) Receive() bool {
	return false
}

func (m *MockSubscribeServerStreamForReconnectTest) Msg() *defangv1.SubscribeResponse {
	return nil
}

func (m *MockSubscribeServerStreamForReconnectTest) Err() error {
	if m.retry < 5 {
		m.retry++
		return m.Error
	}
	return connect.NewError(connect.CodeCanceled, errors.New("cancel connect error")) // cancel the connection after 5 retries to avoid infinite loop
}

type mockSubscribeProviderForReconnectTest struct {
	client.MockProvider
	stream *MockSubscribeServerStreamForReconnectTest
}

func (m *mockSubscribeProviderForReconnectTest) Subscribe(
	_ context.Context,
	_ *defangv1.SubscribeRequest,
) (client.ServerStream[defangv1.SubscribeResponse], error) {
	return m.stream, nil
}

func (m *mockSubscribeProviderForReconnectTest) Receive() bool {
	return false
}

func (m *mockSubscribeProviderForReconnectTest) Err() bool {
	return false
}

func TestWaitServiceStateStreamReceive(t *testing.T) {
	tests := []struct {
		name        string
		stream      *MockSubscribeServerStreamForReconnectTest
		expectRetry bool
	}{
		{
			name: "stream receive returns permission denied error and not retry to connect",
			stream: &MockSubscribeServerStreamForReconnectTest{
				Error: connect.NewError(connect.CodePermissionDenied, errors.New("Not Transient Error")),
			},
			expectRetry: false,
		},
		{
			name: "stream receive returns unavailable error and retry to connect",
			stream: &MockSubscribeServerStreamForReconnectTest{
				Error: connect.NewError(connect.CodeUnavailable, errors.New("stream error")),
			},
			expectRetry: true,
		},
		{
			name: "stream receive returns internal error and retry to connect",
			stream: &MockSubscribeServerStreamForReconnectTest{
				Error: connect.NewError(connect.CodeInternal, errors.New("internal error")),
			},
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			provider := &mockSubscribeProviderForReconnectTest{stream: tt.stream}
			err := WaitServiceState(
				ctx, provider,
				defangv1.ServiceState_DEPLOYMENT_COMPLETED,
				"EtagSomething",
				[]string{"service1"},
			)
			if !tt.expectRetry && isTransientError(err) && provider.stream.retry > 5 {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectRetry && err == nil && provider.stream.retry < 5 {
				t.Error("expected error but got nil")
			}
		})
	}
}
