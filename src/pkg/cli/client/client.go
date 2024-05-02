package client

import (
	"context"

	compose "github.com/compose-spec/compose-go/v2/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

type ServerStream[Res any] interface {
	Close() error
	Receive() bool
	Msg() *Res
	Err() error
}

type ETag = string

type Client interface {
	// Promote(google.protobuf.Empty) returns (google.protobuf.Empty);
	// Subscribe(context.Context, *v1.SubscribeRequest) (*v1.SubscribeResponse, error)
	// Update(context.Context, *v1.Service) (*v1.ServiceInfo, error)
	AgreeToS(context.Context) error
	BootstrapCommand(context.Context, string) (ETag, error)
	BootstrapList(context.Context) error
	CheckLoginAndToS(context.Context) error
	CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error)
	DelegateSubdomainZone(context.Context, *defangv1.DelegateSubdomainZoneRequest) (*defangv1.DelegateSubdomainZoneResponse, error)
	// Deprecated: Use Deploy or Destroy instead.
	Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error)
	DeleteSecrets(context.Context, *defangv1.Secrets) error
	DeleteSubdomainZone(context.Context) error
	Deploy(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	Destroy(context.Context) (ETag, error)
	GenerateFiles(context.Context, *defangv1.GenerateFilesRequest) (*defangv1.GenerateFilesResponse, error)
	Get(context.Context, *defangv1.ServiceID) (*defangv1.ServiceInfo, error)
	GetDelegateSubdomainZone(context.Context) (*defangv1.DelegateSubdomainZoneResponse, error)
	GetServices(context.Context) (*defangv1.ListServicesResponse, error)
	GetVersions(context.Context) (*defangv1.Version, error)
	ListSecrets(context.Context) (*defangv1.Secrets, error)
	Publish(context.Context, *defangv1.PublishRequest) error
	PutSecret(context.Context, *defangv1.SecretValue) error
	Restart(context.Context, ...string) error
	RevokeToken(context.Context) error
	ServiceDNS(name string) string
	Tail(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	TearDown(context.Context) error
	Token(context.Context, *defangv1.TokenRequest) (*defangv1.TokenResponse, error)
	Track(string, ...Property) error
	WhoAmI(context.Context) (*defangv1.WhoAmIResponse, error)

	LoadCompose() (*compose.Project, error)
}

type Property struct {
	Name  string
	Value any
}
