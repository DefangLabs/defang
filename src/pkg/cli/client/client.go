package client

import (
	"context"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

type Client interface {
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

type Property struct {
	Name  string
	Value any
}

type ErrNotImplemented string

func (n ErrNotImplemented) Error() string {
	return string(n)
}
