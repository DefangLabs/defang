package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
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
	ProviderGCP    ProviderID = "gcp"
	// ProviderAzure  ProviderID = "azure"
)

var allProviders = []ProviderID{
	ProviderAuto,
	ProviderDefang,
	ProviderAWS,
	ProviderDO,
	ProviderGCP,
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
	case ProviderGCP:
		return "Google Cloud Platform"
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
	case ProviderGCP:
		return defangv1.Provider_GCP
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
	case defangv1.Provider_GCP:
		*p = ProviderGCP
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

type ServerStream[Res any] interface {
	Close() error
	Receive() bool
	Msg() *Res
	Err() error
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
	DelayBeforeRetry(context.Context) error
	Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error)
	QueryLogs(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error)
	GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error)
	GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error)
	ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error)
	QueryForDebug(context.Context, *defangv1.DebugRequest) error
	Preview(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	PutConfig(context.Context, *defangv1.PutConfigRequest) error
	RemoteProjectName(context.Context) (string, error)
	ServiceDNS(string) string
	SetCanIUseConfig(*defangv1.CanIUseResponse)
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

type RetryDelayer struct {
	Delay time.Duration
}

func (r *RetryDelayer) DelayBeforeRetry(ctx context.Context) error {
	if r.Delay == 0 {
		r.Delay = 1 * time.Second // Minimum 1 second delay to be consistent with the old behavior
	}
	return pkg.SleepWithContext(ctx, r.Delay)
}
