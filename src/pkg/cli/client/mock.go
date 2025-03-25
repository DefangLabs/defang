package client

import (
	"context"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MockProvider struct {
	Provider
	UploadUrl    string
	ServerStream ServerStream[defangv1.TailResponse]
}

func (m MockProvider) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return &defangv1.UploadURLResponse{Url: m.UploadUrl + req.Digest}, nil
}

func (m MockProvider) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{Names: []string{"VAR1"}}, nil
}

func (m MockProvider) ServiceDNS(service string) string {
	return service
}

type MockServerStream struct {
	state int
	Resps []*defangv1.TailResponse
	Error error
}

func (m *MockServerStream) Close() error {
	return nil
}

func (m *MockServerStream) Receive() bool {
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

func (m *MockServerStream) Msg() *defangv1.TailResponse {
	if len(m.Resps) == 0 {
		return nil
	}
	return m.Resps[0]
}

func (m *MockServerStream) Err() error {
	return m.Error
}

type MockFabricClient struct {
	FabricClient
	DelegateDomain string
}

func (m MockFabricClient) CanIUse(ctx context.Context, req *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	return &defangv1.CanIUseResponse{CdImage: "beta", Gpu: true}, nil
}

func (m MockFabricClient) GetDelegateSubdomainZone(ctx context.Context) (*defangv1.DelegateSubdomainZoneResponse, error) {
	return &defangv1.DelegateSubdomainZoneResponse{Zone: m.DelegateDomain}, nil
}

func (m MockFabricClient) DeleteSubdomainZone(ctx context.Context) error {
	return nil
}

func (m MockFabricClient) DelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error) {
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
				Provider:          "aws",
				ProviderAccountId: "1234567890",
				Timestamp:         timestamppb.Now(),
			},
		},
	}, nil
}

func (m MockFabricClient) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	return &defangv1.WhoAmIResponse{
		Tier: defangv1.SubscriptionTier_PERSONAL,
	}, nil
}

type MockLoader struct {
	Project *composeTypes.Project
}

func (m MockLoader) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	return m.Project, nil
}

func (m MockLoader) LoadProjectName(ctx context.Context) (string, error) {
	return m.Project.Name, nil
}
