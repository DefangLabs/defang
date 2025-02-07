package cli

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
)

type mockSubscribeProvider struct {
	client.MockProvider
	ServerStreams []client.ServerStream[defangv1.TailResponse]
	Reqs          []*defangv1.TailRequest
}

type MockSubscribeServerStream struct {
	state int
	Resps []*defangv1.SubscribeResponse
	Error error
}

func (m *MockSubscribeServerStream) Close() error {
	return nil
}

func (m *MockSubscribeServerStream) Receive() bool {
	if len(m.Resps) == 0 {
		return false
	}
	if m.state == 0 {
		m.state = 1
	} else {
		m.Resps = m.Resps[1:]
	}
	return true
}

func (m *MockSubscribeServerStream) Msg() *defangv1.SubscribeResponse {
	if len(m.Resps) == 0 {
		return nil
	}
	return m.Resps[0]
}

func (m *MockSubscribeServerStream) Err() error {
	return m.Error
}

func (d mockSubscribeProvider) Subscribe(_ context.Context, _ *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	s := &MockSubscribeServerStream{
		Resps: []*defangv1.SubscribeResponse{
			{Name: "service1", Status: "COMPLETED", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
			{Name: "service2", Status: "BUILD_FAILED", State: defangv1.ServiceState_BUILD_FAILED},
			{Name: "service3", Status: "COMPLETED", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
		},
	}
	return s, nil
}

func TestWaitServiceState(t *testing.T) {
	ctx := context.Background()
	fabricServer := &grpcListSecretsMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(fabricServer)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	const targetState = defangv1.ServiceState_DEPLOYMENT_COMPLETED
	provider := &mockSubscribeProvider{}

	t.Run("WaitServiceState will succeed", func(t *testing.T) {
		var unmanagedServices = []string{"service1", "service3"}

		fmt.Println(provider)
		err := WaitServiceState(ctx, provider, targetState, "EtagSomething", unmanagedServices)
		if err != nil {
			t.Fatalf("WaitServiceState() failed: %v", err)
		}
	})

	t.Run("WaitServiceState will fail", func(t *testing.T) {
		var unmanagedServices = []string{"service2"}
		err := WaitServiceState(ctx, provider, targetState, "EtagSomething", unmanagedServices)
		if err == nil {
			t.Fatalf("WaitServiceState() failed: %v", err)
		}
	})
}
