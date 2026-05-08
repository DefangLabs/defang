package scaleway

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ByocScaleway struct {
	*byoc.ByocBaseClient

	accessKey string
	secretKey string
	projectID string
	region    string
}

var _ client.Provider = (*ByocScaleway)(nil)

func NewByocProvider(ctx context.Context, tenantLabel types.TenantLabel, stack string) *ByocScaleway {
	b := &ByocScaleway{
		region: os.Getenv("SCW_DEFAULT_REGION"),
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantLabel, b, stack)
	return b
}

func (*ByocScaleway) Driver() string {
	return "scaleway-jobs"
}

func (b *ByocScaleway) Authenticate(ctx context.Context, interactive bool) error {
	b.accessKey = os.Getenv("SCW_ACCESS_KEY")
	b.secretKey = os.Getenv("SCW_SECRET_KEY")
	b.projectID = os.Getenv("SCW_DEFAULT_PROJECT_ID")
	if b.region == "" {
		b.region = "fr-par"
	}

	if b.accessKey == "" || b.secretKey == "" {
		return errors.New("SCW_ACCESS_KEY and SCW_SECRET_KEY must be set (https://www.scaleway.com/en/docs/identity-and-access-management/iam/how-to/create-api-keys/)")
	}
	if b.projectID == "" {
		return errors.New("SCW_DEFAULT_PROJECT_ID must be set (https://www.scaleway.com/en/docs/identity-and-access-management/organizations-and-projects/how-to/create-a-project/)")
	}
	return nil
}

func (b *ByocScaleway) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
	if b.projectID == "" {
		b.projectID = os.Getenv("SCW_DEFAULT_PROJECT_ID")
	}
	if b.projectID == "" {
		return nil, errors.New("SCW_DEFAULT_PROJECT_ID must be set")
	}
	region := b.region
	if region == "" {
		region = os.Getenv("SCW_DEFAULT_REGION")
	}
	return &client.AccountInfo{
		AccountID: b.projectID,
		Provider:  client.ProviderScaleway,
		Region:    region,
	}, nil
}

// ProjectBackend methods

func (b *ByocScaleway) CdCommand(ctx context.Context, req client.CdCommandRequest) (*client.CdCommandResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway CdCommand")
}

func (b *ByocScaleway) CdList(ctx context.Context, _ bool) (iter.Seq[state.Info], error) {
	return nil, client.ErrNotImplemented("Scaleway CdList")
}

func (b *ByocScaleway) GetPrivateDomain(projectName string) string {
	return fmt.Sprintf("%s.internal", projectName)
}

func (b *ByocScaleway) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	return nil, client.ErrNotImplemented("Scaleway GetProjectUpdate")
}

// Provider interface methods

func (b *ByocScaleway) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway CreateUploadURL")
}

func (b *ByocScaleway) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	return client.ErrNotImplemented("Scaleway DeleteConfig")
}

func (b *ByocScaleway) Deploy(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway Deploy")
}

func (b *ByocScaleway) GetDeploymentStatus(ctx context.Context) (bool, error) {
	return false, client.ErrNotImplemented("Scaleway GetDeploymentStatus")
}

func (b *ByocScaleway) GetService(ctx context.Context, req *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
	return nil, client.ErrNotImplemented("Scaleway GetService")
}

func (b *ByocScaleway) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway GetServices")
}

func (b *ByocScaleway) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	return nil, client.ErrNotImplemented("Scaleway ListConfig")
}

func (b *ByocScaleway) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway PrepareDomainDelegation")
}

func (b *ByocScaleway) Preview(ctx context.Context, req *client.DeployRequest) (*client.DeployResponse, error) {
	return nil, client.ErrNotImplemented("Scaleway Preview")
}

func (b *ByocScaleway) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	return client.ErrNotImplemented("Scaleway PutConfig")
}

func (b *ByocScaleway) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (iter.Seq2[*defangv1.TailResponse, error], error) {
	return nil, client.ErrNotImplemented("Scaleway QueryLogs")
}

func (b *ByocScaleway) SetUpCD(ctx context.Context, force bool) error {
	return client.ErrNotImplemented("Scaleway SetUpCD")
}

func (b *ByocScaleway) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	return nil, client.ErrNotImplemented("Scaleway Subscribe")
}

func (b *ByocScaleway) TearDownCD(ctx context.Context) error {
	return client.ErrNotImplemented("Scaleway TearDownCD")
}
