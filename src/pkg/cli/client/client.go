package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
)

type FabricClient interface {
	AgreeToS(context.Context) error
	CanIUse(context.Context, *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error)
	CheckLoginAndToS(context.Context) error
	CreateDelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error)
	Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error)
	DeleteSubdomainZone(context.Context, *defangv1.DeleteSubdomainZoneRequest) error
	Estimate(context.Context, *defangv1.EstimateRequest) (*defangv1.EstimateResponse, error)
	GenerateCompose(context.Context, *defangv1.GenerateComposeRequest) (*defangv1.GenerateComposeResponse, error)
	GenerateFiles(context.Context, *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error)
	GetFabricClient() defangv1connect.FabricControllerClient
	GetDelegateSubdomainZone(context.Context, *defangv1.GetDelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error)
	GetPlaygroundProjectDomain(context.Context) (*defangv1.GetPlaygroundProjectDomainResponse, error)
	GetSelectedProvider(context.Context, *defangv1.GetSelectedProviderRequest) (*defangv1.GetSelectedProviderResponse, error)
	GetTenantName() types.TenantLabel
	GetUploadURL(context.Context, *defangv1.GetUploadURLRequest) (*defangv1.GetUploadURLResponse, error)
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
