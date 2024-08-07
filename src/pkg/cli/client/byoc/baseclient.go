package byoc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/quota"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/consts"
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
	ProjectName             string
	PulumiStack             string
	Quota                   quota.Quotas
	SetupDone               bool
	ShouldDelegateSubdomain bool
	TenantID                string

	project         *compose.Project
	bootstrapLister BootstrapLister
}

func NewByocBaseClient(ctx context.Context, grpcClient client.GrpcClient, tenantID types.TenantID, bl BootstrapLister) *ByocBaseClient {
	b := &ByocBaseClient{
		GrpcClient:  grpcClient,
		TenantID:    string(tenantID),
		ProjectName: "",     // To be overwritten by LoadProject
		PulumiStack: "beta", // TODO: make customizable
		Quota: quota.Quotas{
			// These serve mostly to pevent fat-finger errors in the CLI or Compose files
			ServiceQuotas: quota.ServiceQuotas{
				Cpus:       16,
				Gpus:       8,
				MemoryMiB:  65536,
				Replicas:   16,
				ShmSizeMiB: 30720,
			},
			ConfigCount: 20,   // TODO: add validation for this
			ConfigSize:  4096, // TODO: add validation for this
			Ingress:     10,   // TODO: add validation for this
			Services:    40,
		},
		bootstrapLister: bl,
	}
	return b
}

func (b *ByocBaseClient) Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return nil, client.ErrNotImplemented("AI debugging is not yet supported for BYOC")
}

func (b *ByocBaseClient) GetVersions(context.Context) (*defangv1.Version, error) {
	cdVersion := CdImage[strings.LastIndex(CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocBaseClient) LoadProject(ctx context.Context) (*compose.Project, error) {
	if b.project != nil {
		return b.project, nil
	}
	project, err := b.Loader.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	b.project = project
	b.setProjectName(project.Name)

	return project, nil
}

func (b *ByocBaseClient) LoadProjectName(ctx context.Context) (string, error) {
	if b.ProjectName != "" {
		return b.ProjectName, nil
	}
	projectName, err := b.Loader.LoadProjectName(ctx) // Load the project to get the name
	if err != nil {
		if errors.Is(err, types.ErrComposeFileNotFound) {
			return b.loadProjectNameFromRemote(ctx)
		}

		return "", err
	}

	b.setProjectName(projectName)
	return projectName, nil
}

func (b *ByocBaseClient) ServiceDNS(name string) string {
	return DnsSafeLabel(name) // TODO: consider merging this with getPrivateFqdn
}

func (b *ByocBaseClient) loadProjectNameFromRemote(ctx context.Context) (string, error) {
	// Get the list of projects from remote
	projectNames, err := b.bootstrapLister.BootstrapList(ctx)
	if err != nil {
		return "", err
	}
	for i, name := range projectNames {
		projectNames[i] = strings.Split(name, "/")[0] // Remove the stack name
	}

	if len(projectNames) == 0 {
		return "", errors.New("no projects found")
	}
	if len(projectNames) == 1 {
		term.Debug("Using default project: ", projectNames[0])
		b.setProjectName(projectNames[0])
		return projectNames[0], nil
	}

	// When there are multiple projects, take a hint from COMPOSE_PROJECT_NAME environment variable if set
	if projectName, ok := os.LookupEnv(consts.ComposeProjectName); ok {
		if !slices.Contains(projectNames, projectName) {
			return "", fmt.Errorf("project %q specified by COMPOSE_PROJECT_NAME not found", projectName)
		}
		term.Debug("Using project from COMPOSE_PROJECT_NAME environment variable:", projectName)
		b.setProjectName(projectName)
		return projectName, nil
	}

	return "", errors.New("multiple projects found; please go to the correct project directory where the compose file is or set COMPOSE_PROJECT_NAME")
}

func (b *ByocBaseClient) setProjectName(projectName string) {
	b.ProjectName = projectName
	b.PrivateDomain = DnsSafeLabel(b.ProjectName) + ".internal"
}
