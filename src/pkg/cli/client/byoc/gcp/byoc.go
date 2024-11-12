package gcp

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const (
	DefangCDProjectName = "defang-cd"
)

var (
	defaultCDTags = map[string]string{
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
	b := &ByocGcp{driver: &gcp.Gcp{Region: region}}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantId, b)
	return b
}

func (b *ByocGcp) setUpCD(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// FIXME: Handle organizations

	// 1. Setup project for CD
	cdProj, err := b.driver.EnsureProjectExists(ctx, DefangCDProjectName)
	if err != nil {
		return err
	}

	// 2. Setup cd bucket
	_, err = b.driver.EnsureBucketExists(ctx, cdProj.ProjectId, "defang-cd")
	if err != nil {
		return err
	}

	// 3. Setup Artifact Registry

	b.setupDone = true
	return nil
}

func (b *ByocGcp) BootstrapList(ctx context.Context) ([]string, error) {
	// FIXME: implement
	return nil, client.ErrNotImplemented("GCP bootstrap list")
}
