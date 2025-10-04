package client

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

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
	AccountInfo(context.Context) (*AccountInfo, error)
	BootstrapCommand(context.Context, BootstrapCommandRequest) (types.ETag, error)
	BootstrapList(context.Context) ([]string, error)
	CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error)
	DelayBeforeRetry(context.Context) error
	Delete(context.Context, *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error)
	DeleteConfig(context.Context, *defangv1.Secrets) error
	Deploy(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	Destroy(context.Context, *defangv1.DestroyRequest) (types.ETag, error)
	GetDeploymentStatus(context.Context) error // nil means deployment is pending/running; io.EOF means deployment is done
	GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error)
	GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error)
	GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error)
	ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error)
	PrepareDomainDelegation(context.Context, PrepareDomainDelegationRequest) (*PrepareDomainDelegationResponse, error)
	Preview(context.Context, *defangv1.DeployRequest) (*defangv1.DeployResponse, error)
	PutConfig(context.Context, *defangv1.PutConfigRequest) error
	QueryForDebug(context.Context, *defangv1.DebugRequest) error
	QueryLogs(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	RemoteProjectName(context.Context) (string, error)
	ServicePrivateDNS(string) string
	ServicePublicDNS(string, string) string
	SetCanIUseConfig(*defangv1.CanIUseResponse)
	Subscribe(context.Context, *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error)
	TearDown(context.Context) error
}

type Loader interface {
	LoadProject(context.Context) (*composeTypes.Project, error)
	LoadProjectName(context.Context) (string, error)
}

type RetryDelayer struct {
	Delay    time.Duration
	MaxDelay time.Duration
}

func (r *RetryDelayer) DelayBeforeRetry(ctx context.Context) error {
	if r.Delay == 0 {
		r.Delay = 1 * time.Second // Minimum 1 second delay to be consistent with the old behavior
	}
	if r.MaxDelay == 0 {
		r.MaxDelay = 1 * time.Minute // Default maximum delay
	}
	if r.Delay < r.MaxDelay {
		r.Delay *= 2 // Exponential backoff
	}
	if r.Delay > r.MaxDelay {
		r.Delay = r.MaxDelay // Cap the delay to MaxDelay
	}
	return pkg.SleepWithContext(ctx, r.Delay)
}
