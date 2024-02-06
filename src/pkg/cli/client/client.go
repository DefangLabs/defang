package client

import (
	"context"

	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

type ServerStream[Res any] interface {
	Close() error
	Receive() bool
	Msg() *Res
	Err() error
}

type Client interface {
	// Promote(google.protobuf.Empty) returns (google.protobuf.Empty);
	// Subscribe(context.Context, *v1.SubscribeRequest) (*v1.SubscribeResponse, error)
	// Update(context.Context, *v1.Service) (*v1.ServiceInfo, error)
	AgreeToS(context.Context) error
	BootstrapCommand(context.Context, string) (string, error)
	CreateUploadURL(context.Context, *v1.UploadURLRequest) (*v1.UploadURLResponse, error)
	DelegateSubdomainZone(context.Context, *v1.DelegateSubdomainZoneRequest) (*v1.DelegateSubdomainZoneResponse, error)
	Delete(context.Context, *v1.DeleteRequest) (*v1.DeleteResponse, error)
	DeleteSubdomainZone(context.Context) error
	Deploy(context.Context, *v1.DeployRequest) (*v1.DeployResponse, error)
	GenerateFiles(context.Context, *v1.GenerateFilesRequest) (*v1.GenerateFilesResponse, error)
	Get(context.Context, *v1.ServiceID) (*v1.ServiceInfo, error)
	GetAccessToken() string
	GetDelegateSubdomainZone(context.Context) (*v1.DelegateSubdomainZoneResponse, error)
	GetFabric() string
	GetServices(context.Context) (*v1.ListServicesResponse, error)
	GetStatus(context.Context) (*v1.Status, error)
	GetVersion(context.Context) (*v1.Version, error)
	ListSecrets(context.Context) (*v1.Secrets, error)
	Publish(context.Context, *v1.PublishRequest) error
	PutSecret(context.Context, *v1.SecretValue) error
	RevokeToken(context.Context) error
	Tail(context.Context, *v1.TailRequest) (ServerStream[v1.TailResponse], error)
	Token(context.Context, *v1.TokenRequest) (*v1.TokenResponse, error)
	Track(string, ...Property) error
	WhoAmI(context.Context) (*v1.WhoAmIResponse, error)
}

type Property struct {
	Name  string
	Value any
}
