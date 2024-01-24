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
	GetStatus(context.Context) (*v1.Status, error)
	GetVersion(context.Context) (*v1.Version, error)
	Token(context.Context, *v1.TokenRequest) (*v1.TokenResponse, error)
	RevokeToken(context.Context) error
	Tail(context.Context, *v1.TailRequest) (ServerStream[v1.TailResponse], error)
	// Update(context.Context, *v1.Service) (*v1.ServiceInfo, error)
	Deploy(context.Context, *v1.DeployRequest) (*v1.DeployResponse, error)
	Get(context.Context, *v1.ServiceID) (*v1.ServiceInfo, error)
	Delete(context.Context, *v1.DeleteRequest) (*v1.DeleteResponse, error)
	Publish(context.Context, *v1.PublishRequest) error
	// Subscribe(context.Context, *v1.SubscribeRequest) (*v1.SubscribeResponse, error)
	// rpc Promote(google.protobuf.Empty) returns (google.protobuf.Empty);
	GetServices(context.Context) (*v1.ListServicesResponse, error)
	GenerateFiles(context.Context, *v1.GenerateFilesRequest) (*v1.GenerateFilesResponse, error)
	PutSecret(context.Context, *v1.SecretValue) error
	ListSecrets(context.Context) (*v1.Secrets, error)
	CreateUploadURL(context.Context, *v1.UploadURLRequest) (*v1.UploadURLResponse, error)
	WhoAmI(context.Context) (*v1.WhoAmIResponse, error)

	BootstrapCommand(context.Context, string) error
	GetFabric() string
	GetAccessToken() string
}
