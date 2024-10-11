package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

type ServerStream[Res any] interface {
	Close() error
	Receive() bool
	Msg() *Res
	Err() error
}

type ProjectLoader interface {
	LoadProjectName(context.Context) (string, error)
	LoadProject(context.Context) (*compose.Project, error)
}

type FabricClient interface {
	AgreeToS(context.Context) error
	CheckLoginAndToS(context.Context) error
	Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error)
	DelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error)
	DeleteSubdomainZone(context.Context) error
	GenerateFiles(context.Context, *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error)
	GetDelegateSubdomainZone(context.Context) (*defangv1.DelegateSubdomainZoneResponse, error)
	GetVersions(context.Context) (*defangv1.Version, error)
	Publish(context.Context, *defangv1.PublishRequest) error
	RevokeToken(context.Context) error
	// Subscribe(context.Context, *v1.SubscribeRequest) (*v1.SubscribeResponse, error)
	Token(context.Context, *defangv1.TokenRequest) (*defangv1.TokenResponse, error)
	Track(string, ...Property) error
	VerifyDNSSetup(context.Context, *defangv1.VerifyDNSSetupRequest) error
}

type Client interface {
	FabricClient

	BootstrapCommand(context.Context, string) (types.ETag, error)
	BootstrapList(context.Context) ([]string, error)
	CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error)
	Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error)
	DeleteConfig(context.Context, *defangv1.Secrets) error
	Deploy(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	Destroy(context.Context) (types.ETag, error)
	GetService(context.Context, *defangv1.ServiceID) (*defangv1.ServiceInfo, error)
	GetServices(context.Context) (*defangv1.ListServicesResponse, error)
	ListConfig(context.Context) (*defangv1.Secrets, error)
	PutConfig(context.Context, *defangv1.PutConfigRequest) error
	ServiceDNS(name string) string
	Subscribe(context.Context, *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error)
	Follow(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	TearDown(context.Context) error
	WhoAmI(context.Context) (*defangv1.WhoAmIResponse, error)

	LoadProject(context.Context) (*compose.Project, error)
	LoadProjectName(context.Context) (string, error)
	SetProjectName(string)
}

type Property struct {
	Name  string
	Value any
}

type ErrNotImplemented string

func (n ErrNotImplemented) Error() string {
	return string(n)
}
