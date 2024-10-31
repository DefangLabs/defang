package byoc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/quota"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

const (
	CdDefaultImageTag = "public-beta" // for when a project has no cd version, this would be a old deployment
	CdLatestImageTag  = "public-beta" // Update this to the latest CD service major version number whenever cd major is changed
	CdTaskPrefix      = "defang-cd"   // WARNING: renaming this practically deletes the Pulumi state
)

var (
	DefangPrefix = pkg.Getenv("DEFANG_PREFIX", "Defang") // prefix for all resources created by Defang
)

func AnnotateAwsError(err error) error {
	if err == nil {
		return nil
	}
	term.Debug("AWS error:", err)
	if strings.Contains(err.Error(), "get credentials:") {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if aws.IsS3NoSuchKeyError(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if aws.IsParameterNotFoundError(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return err
}

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

	project         *composeTypes.Project
	bootstrapLister BootstrapLister
}

func NewByocBaseClient(ctx context.Context, grpcClient client.GrpcClient, tenantID types.TenantID, bl BootstrapLister) *ByocBaseClient {
	b := &ByocBaseClient{
		GrpcClient:  grpcClient,
		TenantID:    string(tenantID),
		ProjectName: "",     // To be overwritten by LoadProject
		PulumiStack: "beta", // TODO: make customizable
		Quota: quota.Quotas{
			// These serve mostly to prevent fat-finger errors in the CLI or Compose files
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

func MakeEnv(key string, value any) string {
	return fmt.Sprintf("%s=%q", key, value)
}

func runLocalCommand(ctx context.Context, dir string, env []string, cmd ...string) error {
	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Dir = dir
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func DebugPulumi(ctx context.Context, env []string, cmd ...string) error {
	// Locally we use the "dev" script from package.json to run Pulumi commands, which uses ts-node
	localCmd := append([]string{"npm", "run", "dev"}, cmd...)
	term.Debug(strings.Join(append(env, localCmd...), " "))

	dir := os.Getenv("DEFANG_PULUMI_DIR")
	if dir == "" {
		return nil // show the shell command, but use regular Pulumi command in cloud task
	}

	// Run the Pulumi command locally
	env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"USER=" + os.Getenv("USER"), // needed for Pulumi
	}, env...)
	if err := runLocalCommand(ctx, dir, env, localCmd...); err != nil {
		return err
	}
	// We always return an error to stop the CLI from "tailing" the cloud logs
	return errors.New("local pulumi command succeeded; stopping")
}

func GetCdImage(repo string, tag string) string {
	return pkg.Getenv("DEFANG_CD_IMAGE", repo+":"+tag)
}

func ExtractImageTag(fullQualifiedImageURI string) string {
	index := strings.LastIndex(fullQualifiedImageURI, ":")
	return fullQualifiedImageURI[index+1:]
}

func (b *ByocBaseClient) Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return nil, client.ErrNotImplemented("AI debugging is not yet supported for BYOC")
}

func (b *ByocBaseClient) GetVersions(context.Context) (*defangv1.Version, error) {
	// we want only the latest version of the CD service this CLI was compiled to expect
	return &defangv1.Version{Fabric: CdLatestImageTag}, nil
}

func (b *ByocBaseClient) LoadProject(ctx context.Context) (*composeTypes.Project, error) {
	if b.project != nil {
		return b.project, nil
	}
	project, err := b.Loader.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	b.project = project
	b.SetProjectName(project.Name)

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

	b.SetProjectName(projectName)
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
		b.SetProjectName(projectNames[0])
		return projectNames[0], nil
	}

	term.Warn("Multiple projects found: ", projectNames)

	return "", errors.New("use the --project-name flag to specify a project")
}

func (b *ByocBaseClient) SetProjectName(projectName string) {
	b.ProjectName = projectName
	b.PrivateDomain = DnsSafeLabel(b.ProjectName) + ".internal"
}
