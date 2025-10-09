package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type ProjectLoader interface {
	LoadProjectName(context.Context) (string, error)
	LoadProject(context.Context) (*composeTypes.Project, error)
}

type FabricClient interface {
	AgreeToS(context.Context) error
	CanIUse(context.Context, *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error)
	CheckLoginAndToS(context.Context) error
	Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error)
	DelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error)
	DeleteSubdomainZone(context.Context, *defangv1.DeleteSubdomainZoneRequest) error
	Estimate(context.Context, *defangv1.EstimateRequest) (*defangv1.EstimateResponse, error)
	GenerateCompose(context.Context, *defangv1.GenerateComposeRequest) (*defangv1.GenerateComposeResponse, error)
	GenerateFiles(context.Context, *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error)
	GetController() defangv1connect.FabricControllerClient
	GetDelegateSubdomainZone(context.Context, *defangv1.GetDelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error)
	GetShard(context.Context) (*defangv1.GetShardResponse, error)
	GetSelectedProvider(context.Context, *defangv1.GetSelectedProviderRequest) (*defangv1.GetSelectedProviderResponse, error)
	GetTenantName() types.TenantName
	GetVersions(context.Context) (*defangv1.Version, error)
	ListDeployments(context.Context, *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error)
	Preview(context.Context, *defangv1.PreviewRequest) (*defangv1.PreviewResponse, error)
	Publish(context.Context, *defangv1.PublishRequest) error
	PutDeployment(context.Context, *defangv1.PutDeploymentRequest) error
	RevokeToken(context.Context) error
	SetSelectedProvider(context.Context, *defangv1.SetSelectedProviderRequest) error
	// Subscribe(context.Context, *v1.SubscribeRequest) (*v1.SubscribeResponse, error)
	Token(context.Context, *defangv1.TokenRequest) (*defangv1.TokenResponse, error)
	Track(string, ...Property) error
	VerifyDNSSetup(context.Context, *defangv1.VerifyDNSSetupRequest) error
	WhoAmI(context.Context) (*defangv1.WhoAmIResponse, error)
}

type Property struct {
	Name  string
	Value any
}

type ErrNotImplemented string

func (n ErrNotImplemented) Error() string {
	return string(n)
}
