package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type ProviderID string

const (
	ProviderAuto   ProviderID = "auto"
	ProviderDefang ProviderID = "defang"
	ProviderAWS    ProviderID = "aws"
	ProviderDO     ProviderID = "digitalocean"
	// ProviderGCP    Provider = "gcp"
	// ProviderAzure  Provider = "azure"
)

var allProviders = []ProviderID{
	ProviderAuto,
	ProviderDefang,
	ProviderAWS,
	ProviderDO,
	// ProviderGCP,
	// ProviderAzure,
}

func AllProviders() []ProviderID {
	return allProviders[1:] // skip "auto"
}

func (p ProviderID) String() string {
	return string(p)
}

func (p ProviderID) Name() string {
	switch p {
	case ProviderAuto:
		return "Auto"
	case ProviderDefang:
		return "Defang Playground"
	case ProviderAWS:
		return "AWS"
	case ProviderDO:
		return "DigitalOcean"
	default:
		return p.String()
	}
}

func (p ProviderID) EnumValue() defangv1.Provider {
	switch p {
	case ProviderDefang:
		return defangv1.Provider_DEFANG
	case ProviderAWS:
		return defangv1.Provider_AWS
	case ProviderDO:
		return defangv1.Provider_DIGITALOCEAN
	default:
		return defangv1.Provider_PROVIDER_UNSPECIFIED
	}
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

func (p *ProviderID) SetEnumValue(val defangv1.Provider) {
	switch val {
	case defangv1.Provider_DEFANG:
		*p = ProviderDefang
	case defangv1.Provider_AWS:
		*p = ProviderAWS
	case defangv1.Provider_DIGITALOCEAN:
		*p = ProviderDO
	default:
		*p = ProviderAuto
	}
}

func (p ProviderID) Type() string {
	return "provider"
}

type BootstrapCommandRequest struct {
	Command string
	Project string
}

type PrepareDomainDelegationRequest struct {
	Project        string
	DelegateDomain string
	Preview        bool
}

type PrepareDomainDelegationResponse struct {
	NameServers     []string
	DelegationSetId string
}

type Provider interface {
	AccountInfo(context.Context) (AccountInfo, error)
	BootstrapCommand(context.Context, BootstrapCommandRequest) (types.ETag, error)
	BootstrapList(context.Context) ([]string, error)
	CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error)
	PrepareDomainDelegation(context.Context, PrepareDomainDelegationRequest) (*PrepareDomainDelegationResponse, error)
	Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error)
	DeleteConfig(context.Context, *defangv1.Secrets) error
	Deploy(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error)
	DriverName() string
	Follow(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error)
	GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error)
	ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error)
	Query(context.Context, *defangv1.DebugRequest) error
	Preview(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	PutConfig(context.Context, *defangv1.PutConfigRequest) error
	RemoteProjectName(context.Context) (string, error)
	ServiceDNS(name string) string
	Subscribe(context.Context, *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error)
	TearDown(context.Context) error
}

type AccountInfo interface {
	AccountID() string
	Details() string
	Provider() ProviderID
	Region() string
}

type Loader interface {
	LoadProject(context.Context) (*composeTypes.Project, error)
	LoadProjectName(context.Context) (string, error)
}
