package gcp

import (
	"context"
	"errors"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var _ client.Provider = (*ByocGcp)(nil)

const (
	DefangCDProjectName = "defang-cd"
)

var (
	DefaultCDTags = map[string]string{
		"created-by": "defang",
	}
)

type ByocGcp struct {
	*byoc.ByocBaseClient

	driver    *gcp.Gcp
	setupDone bool
}

func New(ctx context.Context, tenantId types.TenantID) *ByocGcp {
	region := pkg.Getenv("GCP_REGION", "us-central1") // Defaults to us-central1 for lower price
	projectId := os.Getenv("GCP_PROJECT_ID")
	b := &ByocGcp{driver: &gcp.Gcp{Region: region, ProjectId: projectId}}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantId, b)
	return b
}

func (b *ByocGcp) setUpCD(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// TODO: Handle organizations and Project Creation

	// 1. Enable required APIs
	apis := []string{
		"storage.googleapis.com",          // Cloud Storage API
		"artifactregistry.googleapis.com", // Artifact Registry API
		"run.googleapis.com",              // Cloud Run API
	}
	if err := b.driver.EnsureAPIsEnabled(ctx, apis...); err != nil {
		return err
	}

	// 2. Setup cd bucket
	if _, err := b.driver.EnsureBucketExists(ctx, b.driver.ProjectId, "defang-cd"); err != nil {
		return err
	}

	// 3. Setup Artifact Registry
	if _, err := b.driver.EnsureArtifactRegistryExists(ctx, b.driver.ProjectId, b.driver.Region, "defang-cd"); err != nil {
		return err
	}

	// 4. Setup Service Account to be used by cd task in cloud run

	b.setupDone = true
	return nil
}

func (b *ByocGcp) BootstrapList(ctx context.Context) ([]string, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	projectId := os.Getenv("GCP_PROJECT_ID")
	if projectId == "" {
		return nil, errors.New("GCP_PROJECT_ID must be set for GCP projects")
	}
	email, err := b.driver.GetCurrentAccountEmail(ctx)
	if err != nil {
		return nil, err
	}
	return GcpAccountInfo{
		projectId: projectId,
		region:    b.driver.Region,
		email:     email,
	}, nil
}

type GcpAccountInfo struct {
	projectId string
	region    string
	email     string
}

func (g GcpAccountInfo) AccountID() string {
	return g.email
}

func (g GcpAccountInfo) Region() string {
	return g.region
}

func (g GcpAccountInfo) Details() string {
	return g.projectId
}

func (b *ByocGcp) BootstrapCommand(ctx context.Context, req client.BootstrapCommandRequest) (types.ETag, error) {
	if err := b.setUpCD(ctx); err != nil {
		return "", err
	}
	cmd := cdCommand{
		Project: req.Project,
		Command: req.Command,
	}
	cdTaskId, err := b.runCdCommand(ctx, cmd) // TODO: make domain optional for defang cd
	if err != nil {
		return "", err
	}
	return cdTaskId, nil
}

type cdCommand struct {
	Project string
	Command string
}

func (b *ByocGcp) runCdCommand(ctx context.Context, cmd cdCommand) (string, error) {
	return "", client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}
func (b *ByocGcp) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Destroy(ctx context.Context, req *defangv1.DestroyRequest) (types.ETag, error) {
	// FIXME: implement
	return "", client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) GetService(ctx context.Context, req *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.ListServicesResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Query(ctx context.Context, req *defangv1.DebugRequest) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}

func (b *ByocGcp) TearDown(ctx context.Context) error {
	// FIXME: implement
	return client.ErrNotImplemented("GCP bootstrap list")
}
