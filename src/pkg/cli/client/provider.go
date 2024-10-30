package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ProviderID string

const (
	ProviderAuto   ProviderID = "auto"
	ProviderDefang ProviderID = "defang"
	ProviderAWS    ProviderID = "aws"
	ProviderDO     ProviderID = "digitalocean"
	// ProviderAzure  Provider = "azure"
	// ProviderGCP    Provider = "gcp"
)

var allProviders = []ProviderID{
	ProviderAuto,
	ProviderDefang,
	ProviderAWS,
	ProviderDO,
	// ProviderAzure,
	// ProviderGCP,
}

func AllProviders() []ProviderID {
	return allProviders[1:] // skip "auto"
}

func (p ProviderID) String() string {
	return string(p)
}

func (p *ProviderID) Set(str string) error {
	str = strings.ToLower(str)
	for _, provider := range allProviders {
		if provider.String() == str {
			*p = provider
			return nil
		}
	}

	return fmt.Errorf("provider not one of %v", allProviders)
}

func (p ProviderID) Type() string {
	return "provider"
}

type Provider interface {
	AccountInfo(context.Context) (AccountInfo, error)
	BootstrapCommand(context.Context, string, string, string) (types.ETag, error)
	BootstrapList(context.Context) ([]string, error)
	CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error)
	PopulateDebugRequest(context.Context, *defangv1.DebugRequest) error // FIXME: Provider should provide debug context
	Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error)
	DeleteConfig(context.Context, *defangv1.Secrets) error
	Deploy(context.Context, *defangv1.DeployRequest, string) (*defangv1.DeployResponse, error)
	Destroy(ctx context.Context, projectName, delegateDomain string) (types.ETag, error)
	Follow(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	GetService(context.Context, *defangv1.ServiceID, string) (*defangv1.ServiceInfo, error)
	GetServices(context.Context, string) (*defangv1.ListServicesResponse, error)
	ListConfig(context.Context, string) (*defangv1.Secrets, error)
	Preview(context.Context, *defangv1.DeployRequest, string) (*defangv1.DeployResponse, error)
	PutConfig(context.Context, *defangv1.PutConfigRequest) error
	ServiceDNS(name string) string
	Subscribe(context.Context, *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error)
	TearDown(context.Context) error

	// LoadProject(context.Context) (*composeTypes.Project, error)
	// LoadProjectName(context.Context) (string, error)
	// SetProjectName(string)

	RemoteProjectName(context.Context) (string, error)
}

type AccountInfo interface {
	AccountID() string
	Region() string
	Details() string
}
