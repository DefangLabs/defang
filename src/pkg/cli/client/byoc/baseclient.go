package byoc

import (
	"context"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/quota"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

const (
	CdTaskPrefix = "defang-cd" // WARNING: renaming this practically deletes the Pulumi state
	DefangPrefix = "Defang"    // prefix for all resources created by Defang
)

var (
	// Changing this will cause issues if two clients with different versions are using the same account
	CdImage = pkg.Getenv("DEFANG_CD_IMAGE", "public.ecr.aws/defang-io/cd:public-beta")
)

// This function was copied from Fabric controller and slightly modified to work with BYOC
func DnsSafeLabel(fqn string) string {
	return strings.ReplaceAll(DnsSafe(fqn), ".", "-")
}

func DnsSafe(fqdn string) string {
	return strings.ToLower(fqdn)
}

type ByocBaseClient struct {
	client.GrpcClient

	PrivateDomain           string
	PrivateLbIps            []string // TODO: use API to get these
	PrivateNatIps           []string // TODO: use API to get these
	ProjectDomain           string
	PulumiProject           string
	PulumiStack             string
	Quota                   quota.Quotas
	SetupDone               bool
	ShouldDelegateSubdomain bool
	TenantID                string

	loadProjOnce func() (*compose.Project, error)
}

func NewByocBaseClient(ctx context.Context, grpcClient client.GrpcClient, tenantID types.TenantID) *ByocBaseClient {
	b := &ByocBaseClient{
		GrpcClient:    grpcClient,
		TenantID:      string(tenantID),
		PulumiProject: "",     // To be overwritten by LoadProject
		PulumiStack:   "beta", // TODO: make customizable
		Quota: quota.Quotas{
			// These serve mostly to pevent fat-finger errors in the CLI or Compose files
			Cpus:       16,
			Gpus:       8,
			MemoryMiB:  65536,
			Replicas:   16,
			Services:   40,
			ShmSizeMiB: 30720,
		},
	}
	b.loadProjOnce = sync.OnceValues(func() (*compose.Project, error) {
		proj, err := b.GrpcClient.Loader.LoadCompose(ctx)
		if err != nil {
			return nil, err
		}
		b.PrivateDomain = DnsSafeLabel(proj.Name) + ".internal"
		b.PulumiProject = proj.Name
		return proj, nil
	})
	return b
}

func (b *ByocBaseClient) GetVersions(context.Context) (*defangv1.Version, error) {
	cdVersion := CdImage[strings.LastIndex(CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocBaseClient) LoadProject(ctx context.Context) (*compose.Project, error) {
	return b.loadProjOnce()
}

func (b *ByocBaseClient) ServiceDNS(name string) string {
	return DnsSafeLabel(name) // TODO: consider merging this with getPrivateFqdn
}
