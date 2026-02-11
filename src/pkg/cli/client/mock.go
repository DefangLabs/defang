package client

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"path"
	"sync"
	"sync/atomic"

	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MockIter creates an iter.Seq2 from a pre-populated list of responses and a final error.
// A nil response in the list acts as a stream-end marker.
func MockIter[T any](resps []*T, finalErr error) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		for _, resp := range resps {
			if resp == nil {
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
		if finalErr != nil {
			yield(nil, finalErr)
		}
	}
}

type MockProvider struct {
	Provider
	UploadUrl string
}

func (m MockProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	url, err := url.JoinPath(m.UploadUrl, req.Project, req.Stack, req.Digest)
	return &defangv1.UploadURLResponse{Url: url}, err
}

func (m MockProvider) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{Names: []string{"CONFIG1", "CONFIG2", "dummy", "ENV1", "SENSITIVE_DATA", "VAR1"}}, nil
}

func (m MockProvider) ServicePrivateDNS(service string) string {
	return "mock-" + service
}

func (m MockProvider) ServicePublicDNS(service string, projectName string) string {
	return dns.SafeLabel(service) + "." + dns.SafeLabel(projectName) + ".tenant2.defang.app"
}

func (MockProvider) UpdateShardDomain(ctx context.Context) error {
	return nil
}

func (MockProvider) GetStackName() string {
	return "test"
}

func (MockProvider) GetStackNameForDomain() string {
	return ""
}

// MockServerStream mocks a ServerStream.
type MockServerStream[Msg any] struct {
	index  int
	Resps  []*Msg
	Error  error
	closed atomic.Bool
}

func (m *MockServerStream[T]) Close() error {
	if m.closed.Swap(true) {
		panic("MockServerStream already closed")
	}
	return nil
}

func (m *MockServerStream[T]) Receive() bool {
	if m.index >= len(m.Resps) || m.closed.Load() {
		return false
	}
	m.index++
	return m.Resps[m.index-1] != nil
}

func (m *MockServerStream[T]) Msg() *T {
	if m.index == 0 || m.index > len(m.Resps) {
		return nil
	}
	return m.Resps[m.index-1]
}

func (m *MockServerStream[T]) Err() error {
	return m.Error
}

// MockWaitStream is a mock implementation of the ServerStream interface that
// returns messages and errors from channels. It blocks until the channels are
// closed or an error is received. It is used for testing purposes.
type MockWaitStream[T any] struct {
	msg       *T
	err       error
	msgCh     chan *T
	done      chan struct{}
	closeOnce sync.Once
}

// NewMockWaitStream returns a ServerStream that will block until closed.
func NewMockWaitStream[T any]() *MockWaitStream[T] {
	return &MockWaitStream[T]{msgCh: make(chan *T), done: make(chan struct{})}
}

func (m *MockWaitStream[T]) Send(msg *T, err error) {
	m.err = err
	m.msgCh <- msg
}

func (m *MockWaitStream[T]) Receive() bool {
	select {
	case msg, ok := <-m.msgCh:
		m.msg = msg
		return ok && msg != nil
	case <-m.done:
		return false
	}
}

func (m *MockWaitStream[T]) Msg() *T {
	return m.msg
}

func (m *MockWaitStream[T]) Err() error {
	return m.err
}

func (m *MockWaitStream[T]) Close() error {
	m.closeOnce.Do(func() { close(m.done) })
	return nil
}

// ServerStreamIterCtx adapts a ServerStream to iter.Seq2, closing the stream when the
// context is canceled. This is needed for blocking streams (e.g. MockWaitStream)
// where Receive() blocks on a channel and won't return until Close() is called.
func ServerStreamIterCtx[T any](ctx context.Context, stream ServerStream[T]) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		var closeOnce sync.Once
		closeStream := func() { closeOnce.Do(func() { stream.Close() }) }

		// Close the stream when context is canceled to unblock Receive()
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				closeStream()
			case <-done:
			}
		}()
		defer close(done)
		defer closeStream()

		for stream.Receive() {
			if !yield(stream.Msg(), nil) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

type MockFabricClient struct {
	FabricClient
	DelegateDomain string
}

func (m MockFabricClient) GetFabricClient() defangv1connect.FabricControllerClient {
	return defangv1connect.NewFabricControllerClient(http.DefaultClient, "localhost")
}

func (m MockFabricClient) GetPlaygroundProjectDomain(ctx context.Context) (*defangv1.GetPlaygroundProjectDomainResponse, error) {
	return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing bearer token"))
}

func (m MockFabricClient) AgreeToS(ctx context.Context) error {
	return nil
}

func (m MockFabricClient) CanIUse(ctx context.Context, req *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	return &defangv1.CanIUseResponse{CdImage: "beta", Gpu: true}, nil
}

func (m MockFabricClient) GetDelegateSubdomainZone(context.Context, *defangv1.GetDelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return &defangv1.DelegateSubdomainZoneResponse{Zone: m.DelegateDomain}, nil
}

func (m MockFabricClient) DeleteSubdomainZone(context.Context, *defangv1.DeleteSubdomainZoneRequest) error {
	return nil
}

func (m MockFabricClient) CreateDelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return &defangv1.DelegateSubdomainZoneResponse{Zone: "example.com"}, nil
}

func (m MockFabricClient) PutStack(ctx context.Context, req *defangv1.PutStackRequest) error {
	return nil
}

func (m MockFabricClient) PutDeployment(ctx context.Context, req *defangv1.PutDeploymentRequest) error {
	return nil
}

func (m MockFabricClient) ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error) {
	return &defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{
			{
				Id:                "a1b2c3",
				Project:           "test",
				Provider:          defangv1.Provider_AWS,
				ProviderAccountId: "1234567890",
				ProviderString:    "aws",
				Region:            "us-test-2",
				Timestamp:         timestamppb.Now(),
			},
		},
	}, nil
}

func (m MockFabricClient) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	name := req.Digest
	if req.Filename != "" {
		name = req.Filename
	}
	if req.Stack != "" {
		name = path.Join(req.Stack, name)
	}
	if req.Project != "" {
		name = path.Join(req.Project, name)
	}
	return &defangv1.UploadURLResponse{Url: "http://mock-upload-url/" + name}, nil
}

func (m MockFabricClient) GetTenantName() types.TenantLabel {
	return "tenant-mock"
}

func (m MockFabricClient) GetDefaultStack(context.Context, *defangv1.GetDefaultStackRequest) (*defangv1.GetStackResponse, error) {
	return &defangv1.GetStackResponse{
		Stack: &defangv1.Stack{
			Name:    "default",
			Project: "default-project",
		},
	}, nil
}

type MockLoader struct {
	Project composeTypes.Project
	Error   error
}

func (m MockLoader) LoadProject(context.Context) (*composeTypes.Project, error) {
	return &m.Project, m.Error
}

func (m MockLoader) LoadProjectName(context.Context) (string, bool, error) {
	return m.Project.Name, false, m.Error
}

func (m MockLoader) TargetDirectory(context.Context) string {
	return "."
}

func (m MockLoader) CreateProjectForDebug() (*composeTypes.Project, error) {
	return &m.Project, m.Error
}
