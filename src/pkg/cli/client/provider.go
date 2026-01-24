package client

import (
	"context"
	"iter"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type DNSResolver interface {
	ServicePrivateDNS(name string) string
	ServicePublicDNS(name string, projectName string) string
	UpdateShardDomain(ctx context.Context) error
}

type CdCommand string

const (
	CdCommandCancel  CdCommand = "cancel"
	CdCommandDestroy CdCommand = "destroy"
	CdCommandDown    CdCommand = "down"
	CdCommandList    CdCommand = "list"
	CdCommandOutputs CdCommand = "outputs"
	CdCommandPreview CdCommand = "preview" // needs Compose payload
	CdCommandRefresh CdCommand = "refresh"
	CdCommandUp      CdCommand = "up" // needs Compose payload
)

type CdCommandRequest struct {
	Command   CdCommand
	Project   string
	StatesUrl string
	EventsUrl string
}

type DeployRequest struct {
	defangv1.DeployRequest

	StatesUrl string
	EventsUrl string
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
	DNSResolver
	AccountInfo(context.Context) (*AccountInfo, error)
	CdCommand(context.Context, CdCommandRequest) (types.ETag, error)
	CdList(context.Context, bool) (iter.Seq[string], error)
	CreateUploadURL(context.Context, *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error)
	DelayBeforeRetry(context.Context) error
	DeleteConfig(context.Context, *defangv1.Secrets) error
	Deploy(context.Context, *DeployRequest) (*defangv1.DeployResponse, error)
	GetDeploymentStatus(context.Context) error // nil means deployment is pending/running; io.EOF means deployment is done
	GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error)
	GetService(context.Context, *defangv1.GetRequest) (*defangv1.ServiceInfo, error)
	GetServices(context.Context, *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error)
	GetStackName() string
	GetStackNameForDomain() string
	ListConfig(context.Context, *defangv1.ListConfigsRequest) (*defangv1.Secrets, error)
	PrepareDomainDelegation(context.Context, PrepareDomainDelegationRequest) (*PrepareDomainDelegationResponse, error)
	Preview(context.Context, *DeployRequest) (*defangv1.DeployResponse, error)
	PutConfig(context.Context, *defangv1.PutConfigRequest) error
	QueryLogs(context.Context, *defangv1.TailRequest) (ServerStream[defangv1.TailResponse], error)
	// Deprecated: should use stacks instead of ProjectName fallback.
	RemoteProjectName(context.Context) (string, error)
	SetCanIUseConfig(*defangv1.CanIUseResponse)
	SetUpCD(context.Context) error
	Subscribe(context.Context, *defangv1.SubscribeRequest) (ServerStream[defangv1.SubscribeResponse], error)
	TearDownCD(context.Context) error
}

type Loader interface {
	LoadProject(context.Context) (*composeTypes.Project, error)
	LoadProjectName(context.Context) (string, bool, error) // true = name from loaded project
	TargetDirectory() string
	CreateProjectForDebug() (*composeTypes.Project, error)
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
