package byoc

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/quota"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/consts"
	"github.com/compose-spec/compose-go/v2/loader"
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

type BootstrapLister interface {
	BootstrapList(context.Context) ([]string, error)
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

	loadProjOnce    func() (*compose.Project, error)
	bootstrapLister BootstrapLister
}

func NewByocBaseClient(ctx context.Context, grpcClient client.GrpcClient, tenantID types.TenantID, bl BootstrapLister) *ByocBaseClient {
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
		bootstrapLister: bl,
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

func (b *ByocBaseClient) LoadProjectName(ctx context.Context) (string, error) {
	proj, err := b.loadProjOnce()
	if err == nil {
		return proj.Name, nil
	}
	if !errors.Is(err, types.ErrComposeFileNotFound) {
		return "", err
	}

	// Get the project name from the environment (since it overrides the compose file anyway)
	if projectName, ok := os.LookupEnv(consts.ComposeProjectName); ok {
		if projectName != loader.NormalizeProjectName(projectName) {
			return "", loader.InvalidProjectNameErr(projectName)
		}
		b.PulumiProject = projectName
		return projectName, nil
	}

	// Get the list of projects from remote
	projectNames, err := b.bootstrapLister.BootstrapList(ctx)
	if err != nil {
		return "", err
	}
	for i, name := range projectNames {
		projectNames[i] = strings.Split(name, "/")[0] // Remove the stack name
	}

	if len(projectNames) == 0 {
		return "", types.ErrComposeFileNotFound
	}
	if len(projectNames) == 1 {
		term.Debug("Using default project: ", projectNames[0])
		b.PulumiProject = projectNames[0]
		return projectNames[0], nil
	}

	return "", errors.New("multiple projects found; please go to the correct project directory where the compose file is or set COMPOSE_PROJECT_NAME")
}

func (b *ByocBaseClient) ServiceDNS(name string) string {
	return DnsSafeLabel(name) // TODO: consider merging this with getPrivateFqdn
}
