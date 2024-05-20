package byoc

import (
	"context"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/quota"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
	"os"
	"strings"
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
	*client.GrpcClient

	CustomDomain            string //TODO: Not BYOD domain which is per service, should rename to something like delegated defang domain
	PrivateDomain           string
	PrivateLbIps            []string
	PrivateNatIps           []string
	PulumiProject           string
	PulumiStack             string
	Quota                   quota.Quotas
	SetupDone               bool
	TenantID                string
	ShouldDelegateSubdomain bool
}

func NewByocBaseClient(grpcClient *client.GrpcClient, tenantID types.TenantID) *ByocBaseClient {
	return &ByocBaseClient{
		GrpcClient:    grpcClient,
		TenantID:      string(tenantID),
		PulumiProject: os.Getenv("COMPOSE_PROJECT_NAME"),
		PulumiStack:   "beta", // TODO: make customizable
	}
}

func (b ByocBaseClient) GetVersions(context.Context) (*defangv1.Version, error) {
	cdVersion := CdImage[strings.LastIndex(CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b ByocBaseClient) LoadProject() (*compose.Project, error) {
	if b.PrivateDomain != "" {
		panic("LoadProject should only be called once")
	}
	var proj *compose.Project
	var err error

	if b.PulumiProject != "" {
		proj, err = b.GrpcClient.Loader.LoadWithProjectName(b.PulumiProject)
	} else {
		proj, err = b.GrpcClient.Loader.LoadWithDefaultProjectName(b.TenantID)
	}
	if err != nil {
		return nil, err
	}
	b.PrivateDomain = DnsSafeLabel(proj.Name) + ".internal"
	b.PulumiProject = proj.Name
	return proj, nil
}

func (b ByocBaseClient) LoadProjectName() (string, error) {
	if b.PulumiProject != "" {
		return b.PulumiProject, nil
	}
	p, err := b.LoadProject()
	if err != nil {
		return b.TenantID, err
	}
	return p.Name, nil
}

func (b *ByocBaseClient) ServiceDNS(name string) string {
	return DnsSafeLabel(name) // TODO: consider merging this with getPrivateFqdn
}
