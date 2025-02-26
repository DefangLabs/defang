package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type ServerStream[Res any] interface {
	Close() error
	Receive() bool
	Msg() *Res
	Err() error
}

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
	DeleteSubdomainZone(context.Context) error
	GenerateFiles(context.Context, *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error)
	GetController() defangv1connect.FabricControllerClient
	GetDelegateSubdomainZone(context.Context) (*defangv1.DelegateSubdomainZoneResponse, error)
	GetTenantName() types.TenantName
	GetSelectedProvider(context.Context, *defangv1.GetSelectedProviderRequest) (*defangv1.GetSelectedProviderResponse, error)
	GetVersions(context.Context) (*defangv1.Version, error)
	Publish(context.Context, *defangv1.PublishRequest) error
	PutDeployment(context.Context, *defangv1.PutDeploymentRequest) error
	ListDeployments(context.Context, *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error)
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
