package cli

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockSubscribeProvider mocks the provider for Subscribe.
type mockSubscribeProvider struct {
	client.MockProvider
	reqs  []*defangv1.SubscribeRequest
	resps map[types.ETag][]*defangv1.SubscribeResponse
}

func (m *mockSubscribeProvider) Subscribe(
	_ context.Context,
	req *defangv1.SubscribeRequest,
) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	m.reqs = append(m.reqs, req)

	resps, ok := m.resps[req.Etag]
	if !ok {
		panic("unexpected etag; not in resps map")
	}

	return client.MockIter(resps, nil), nil
}

func TestWaitServiceState(t *testing.T) {
	ctx := t.Context()
	provider := &mockSubscribeProvider{
		resps: map[string][]*defangv1.SubscribeResponse{
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
		},
	}

	noErrTests := []struct {
		etag        string
		services    []string
		targetState defangv1.ServiceState
		expected    ServiceStates
	}{
		{
			etag:        "etag1",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			expected: ServiceStates{
				"service1": defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
		},
		{
			etag:        "etag2",
			services:    []string{"service1", "service2"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			expected: ServiceStates{
				"service1": defangv1.ServiceState_DEPLOYMENT_COMPLETED,
				"service2": defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			},
		},
	}

	for _, tt := range noErrTests {
		t.Run("Expect No Error", func(t *testing.T) {
			ss, err := WaitServiceState(ctx, provider, tt.targetState, "testproject", tt.etag, tt.services)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !reflect.DeepEqual(ss, tt.expected) {
				t.Errorf("Expected service states %v, got: %v", tt.expected, ss)
			}
		})
	}

	errTests := []struct {
		etag        string
		services    []string
		targetState defangv1.ServiceState
		expected    ServiceStates
	}{
		{
			etag:        "etag3",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			expected: ServiceStates{
				"service1": defangv1.ServiceState_BUILD_FAILED,
			},
		},
		{
			etag:        "etag4",
			services:    []string{"service1"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			expected: ServiceStates{
				"service1": defangv1.ServiceState_DEPLOYMENT_FAILED,
			},
		},
		{
			etag:        "etag5",
			services:    []string{"service1", "service2", "service3"},
			targetState: defangv1.ServiceState_DEPLOYMENT_COMPLETED,
			expected: ServiceStates{
				"service1": defangv1.ServiceState_DEPLOYMENT_COMPLETED,
				"service2": defangv1.ServiceState_DEPLOYMENT_COMPLETED,
				"service3": defangv1.ServiceState_DEPLOYMENT_FAILED,
			},
		},
	}

	for _, tt := range errTests {
		t.Run("Expect Error", func(t *testing.T) {
			ss, err := WaitServiceState(ctx, provider, tt.targetState, "testproject", tt.etag, tt.services)
			if err == nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !errors.As(err, &client.ErrDeploymentFailed{}) {
				t.Errorf("Expected ErrDeploymentFailed but got: %v", err)
			}
			if !reflect.DeepEqual(ss, tt.expected) {
				t.Errorf("Expected service states %v, got: %v", tt.expected, ss)
			}
		})
	}

	if len(provider.reqs) == 0 {
		t.Errorf("Expected Subscribe to be called but got 0 requests")
	}
}

type mockSubscribeProviderForReconnectTest struct {
	client.MockProvider
	err   error
	retry int
	client.RetryDelayer
}

func (m *mockSubscribeProviderForReconnectTest) Subscribe(
	_ context.Context,
	_ *defangv1.SubscribeRequest,
) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	var err error
	if m.retry < 5 {
		m.retry++
		err = m.err
	} else {
		err = connect.NewError(connect.CodeCanceled, errors.New("cancel connect error"))
	}
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		yield(nil, err)
	}, nil
}

// GetServices returns no services, so the reconnect-time poll never short-circuits
// these tests: they exercise the stream-reconnect path itself, not the poll fallback.
func (m *mockSubscribeProviderForReconnectTest) GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return &defangv1.GetServicesResponse{}, nil
}

func TestWaitServiceStateStreamReceive(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectRetry bool
	}{
		{
			name:        "stream receive returns permission denied error and not retry to connect",
			err:         connect.NewError(connect.CodePermissionDenied, errors.New("Not Transient Error")),
			expectRetry: false,
		},
		{
			name:        "stream receive returns unavailable error and retry to connect",
			err:         connect.NewError(connect.CodeUnavailable, errors.New("stream error")),
			expectRetry: true,
		},
		{
			name:        "stream receive returns internal error and retry to connect",
			err:         connect.NewError(connect.CodeInternal, errors.New("internal error")),
			expectRetry: true,
		},
		{
			name:        "stream receive returns resource exhausted error and retry to connect",
			err:         status.Error(codes.ResourceExhausted, "quota exceeded"),
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			provider := &mockSubscribeProviderForReconnectTest{err: tt.err, RetryDelayer: client.RetryDelayer{Delay: 1 * time.Millisecond}}
			_, err := WaitServiceState(
				ctx, provider,
				defangv1.ServiceState_DEPLOYMENT_COMPLETED,
				"testproject",
				"EtagSomething",
				[]string{"service1"},
			)
			if !tt.expectRetry && isTransientError(err) && provider.retry > 5 {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectRetry && err == nil && provider.retry < 5 {
				t.Error("expected error but got nil")
			}
		})
	}
}

// mockStalledStreamProvider's Subscribe always returns a transient error, simulating a log-tail
// stream that never delivers the health-transition event (e.g. because it happened before the
// stream reconnected, #2241). GetServices reports the true current state, independent of the stream.
type mockStalledStreamProvider struct {
	client.MockProvider
	client.RetryDelayer
	servicesResp    *defangv1.GetServicesResponse
	servicesErr     error
	failFirstNPolls int
	getServicesN    int
}

func (m *mockStalledStreamProvider) Subscribe(
	_ context.Context,
	_ *defangv1.SubscribeRequest,
) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		yield(nil, connect.NewError(connect.CodeUnavailable, errors.New("idle timeout: no data received")))
	}, nil
}

func (m *mockStalledStreamProvider) GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	m.getServicesN++
	if m.getServicesN <= m.failFirstNPolls {
		return nil, m.servicesErr
	}
	return m.servicesResp, nil
}

func TestWaitServiceStatePollFallbackOnStalledStream(t *testing.T) {
	tests := []struct {
		name                string
		servicesResp        *defangv1.GetServicesResponse
		servicesErr         error
		failFirstNPolls     int
		ctxTimeout          time.Duration // 0 means no deadline
		wantErrDeployFailed bool
		wantCtxErr          bool
		wantStates          ServiceStates
	}{
		{
			name: "poll observes target state that the stream can never see again",
			servicesResp: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{Service: &defangv1.Service{Name: "service1"}, Etag: "EtagSomething", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
				},
			},
			wantStates: ServiceStates{"service1": defangv1.ServiceState_DEPLOYMENT_COMPLETED},
		},
		{
			name: "poll ignores services with a mismatched etag",
			servicesResp: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{Service: &defangv1.Service{Name: "service1"}, Etag: "SomeOtherEtag", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
				},
			},
			ctxTimeout: 20 * time.Millisecond,
			wantCtxErr: true,
		},
		{
			name:            "poll error is not fatal; polling continues on later reconnects",
			servicesErr:     errors.New("transient GetServices failure"),
			failFirstNPolls: 2,
			servicesResp: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{Service: &defangv1.Service{Name: "service1"}, Etag: "EtagSomething", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED},
				},
			},
			wantStates: ServiceStates{"service1": defangv1.ServiceState_DEPLOYMENT_COMPLETED},
		},
		{
			name: "poll observes a BUILD_FAILED state",
			servicesResp: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{Service: &defangv1.Service{Name: "service1"}, Etag: "EtagSomething", State: defangv1.ServiceState_BUILD_FAILED, Status: "build failed"},
				},
			},
			wantErrDeployFailed: true,
			wantStates:          ServiceStates{"service1": defangv1.ServiceState_BUILD_FAILED},
		},
		{
			name: "poll observes a DEPLOYMENT_FAILED state",
			servicesResp: &defangv1.GetServicesResponse{
				Services: []*defangv1.ServiceInfo{
					{Service: &defangv1.Service{Name: "service1"}, Etag: "EtagSomething", State: defangv1.ServiceState_DEPLOYMENT_FAILED, Status: "deploy failed"},
				},
			},
			wantErrDeployFailed: true,
			wantStates:          ServiceStates{"service1": defangv1.ServiceState_DEPLOYMENT_FAILED},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.ctxTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.ctxTimeout)
				defer cancel()
			}
			provider := &mockStalledStreamProvider{
				RetryDelayer:    client.RetryDelayer{Delay: 1 * time.Millisecond},
				servicesResp:    tt.servicesResp,
				servicesErr:     tt.servicesErr,
				failFirstNPolls: tt.failFirstNPolls,
			}
			ss, err := WaitServiceState(
				ctx, provider,
				defangv1.ServiceState_DEPLOYMENT_COMPLETED,
				"testproject",
				"EtagSomething",
				[]string{"service1"},
			)

			switch {
			case tt.wantCtxErr:
				if err == nil {
					t.Fatal("expected an error once the context deadline is exceeded, got nil")
				}
			case tt.wantErrDeployFailed:
				if !errors.As(err, &client.ErrDeploymentFailed{}) {
					t.Fatalf("expected ErrDeploymentFailed, got: %v", err)
				}
			default:
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tt.wantStates != nil && !reflect.DeepEqual(ss, tt.wantStates) {
				t.Errorf("expected service states %v, got: %v", tt.wantStates, ss)
			}
			if tt.failFirstNPolls > 0 && provider.getServicesN <= tt.failFirstNPolls {
				t.Errorf("expected polling to continue past the initial error(s), got %d GetServices calls", provider.getServicesN)
			}
		})
	}
}
