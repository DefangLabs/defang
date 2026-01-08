package client

import (
	"context"
	"errors"
	"net/http"
	"path"

	"github.com/DefangLabs/defang/src/pkg/dns"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MockProvider struct {
	Provider
	UploadUrl string
}

func (m MockProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return &defangv1.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
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
	index int
	Resps []*Msg
	Error error
}

func (*MockServerStream[T]) Close() error {
	return nil
}

func (m *MockServerStream[T]) Receive() bool {
	if m.index >= len(m.Resps) {
		return false
	}
	m.index++
	return true
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
	msg   *T
	err   error
	msgCh chan *T
}

// NewMockWaitStream returns a ServerStream that will block until closed.
func NewMockWaitStream[T any]() *MockWaitStream[T] {
	return &MockWaitStream[T]{msgCh: make(chan *T)}
}

func (m *MockWaitStream[T]) Send(msg *T, err error) {
	m.err = err
	m.msgCh <- msg
}

func (m *MockWaitStream[T]) Receive() bool {
	msg, ok := <-m.msgCh
	m.msg = msg
	return ok && msg != nil
}

func (m *MockWaitStream[T]) Msg() *T {
	return m.msg
}

func (m *MockWaitStream[T]) Err() error {
	return m.err
}

func (m *MockWaitStream[T]) Close() error {
	close(m.msgCh)
	return nil
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

type MockLoader struct {
	Project composeTypes.Project
	Error   error
}

func (m MockLoader) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return &m.Project, m.Error
}

func (m MockLoader) LoadProjectName(ctx context.Context) (string, error) {
	return m.Project.Name, m.Error
}

func (m MockLoader) TargetDirectory() string {
	return "."
}
