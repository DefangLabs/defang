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
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const (
	CdDefaultImageTag = "public-beta" // for when a project has no cd version, this would be a old deployment
	CdLatestImageTag  = "public-beta" // Update this to the latest CD service major version number whenever cd major is changed
	CdTaskPrefix      = "defang-cd"   // WARNING: renaming this practically deletes the Pulumi state
)

var (
	DefangPrefix = pkg.Getenv("DEFANG_PREFIX", "Defang") // prefix for all resources created by Defang
)

// This function was copied from Fabric controller and slightly modified to work with BYOC
func DnsSafeLabel(fqn string) string {
	return strings.ReplaceAll(DnsSafe(fqn), ".", "-")
}

func DnsSafe(fqdn string) string {
	return strings.ToLower(fqdn)
}

type ErrMultipleProjects struct {
	ProjectNames []string
}

func (mp ErrMultipleProjects) Error() string {
	return fmt.Sprintf("multiple projects found; use the --project-name flag to specify a project: \n%v", strings.Join(mp.ProjectNames, "\n"))
}

type ProjectBackend interface {
	BootstrapList(context.Context) ([]string, error)
	GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error)
}

type ByocBaseClient struct {
	client.RetryDelayer

	PulumiStack             string
	SetupDone               bool
	ShouldDelegateSubdomain bool
	TenantName              string
	CDImage                 string

	projectBackend ProjectBackend
}

func NewByocBaseClient(ctx context.Context, tenantName types.TenantName, backend ProjectBackend) *ByocBaseClient {
	b := &ByocBaseClient{
		TenantName:     string(tenantName),
		PulumiStack:    "beta", // TODO: make customizable
		projectBackend: backend,
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
		"USER=" + pkg.GetCurrentUser(), // needed for Pulumi
	}, env...)
	if err := runLocalCommand(ctx, dir, env, localCmd...); err != nil {
		return err
	}
	// We always return an error to stop the CLI from "tailing" the cloud logs
	return errors.New("local pulumi command succeeded; stopping")
}

func (b *ByocBaseClient) GetProjectLastCDImage(ctx context.Context, projectName string) (string, error) {
	projUpdate, err := b.projectBackend.GetProjectUpdate(ctx, projectName)
	if err != nil {
		return "", err
	}

	if projUpdate == nil {
		return "", nil
	}

	return projUpdate.CdVersion, nil
}

func ExtractImageTag(fullQualifiedImageURI string) string {
	index := strings.LastIndex(fullQualifiedImageURI, ":")
	return fullQualifiedImageURI[index+1:]
}

func (b *ByocBaseClient) Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return nil, client.ErrNotImplemented("AI debugging is not yet supported for BYOC")
}

func (b *ByocBaseClient) SetCDImage(image string) {
	b.CDImage = image
}

func (b *ByocBaseClient) GetVersions(context.Context) (*defangv1.Version, error) {
	// we want only the latest version of the CD service this CLI was compiled to expect
	return &defangv1.Version{Fabric: CdLatestImageTag}, nil
}

func (b *ByocBaseClient) ServiceDNS(name string) string {
	return DnsSafeLabel(name) // TODO: consider merging this with getPrivateFqdn
}

func (b *ByocBaseClient) RemoteProjectName(ctx context.Context) (string, error) {
	// Get the list of projects from remote
	projectNames, err := b.projectBackend.BootstrapList(ctx)
	if err != nil {
		return "", err
	}
	for i, name := range projectNames {
		projectNames[i] = strings.Split(name, "/")[0] // Remove the stack name
	}

	if len(projectNames) == 0 {
		return "", errors.New("no projects found")
	}

	if len(projectNames) > 1 {
		return "", ErrMultipleProjects{ProjectNames: projectNames}
	}
	term.Debug("Using default project:", projectNames[0])
	return projectNames[0], nil
}

func (b *ByocBaseClient) GetProjectDomain(projectName, zone string) string {
	if projectName == "" {
		return "" // no project name => no custom domain
	}
	projectLabel := DnsSafeLabel(projectName)
	if projectLabel == DnsSafeLabel(b.TenantName) {
		return DnsSafe(zone) // the zone will already have the tenant ID
	}
	return projectLabel + "." + DnsSafe(zone)
}

// stackDir returns a stack-qualified path, like the Pulumi TS function `stackDir`
func (b *ByocBaseClient) StackDir(projectName, name string) string {
	pkg.Ensure(projectName != "", "ProjectName not set")
	return fmt.Sprintf("/%s/%s/%s/%s", DefangPrefix, projectName, b.PulumiStack, name) // same as shared/common.ts
}

func GetPrivateDomain(projectName string) string {
	return DnsSafeLabel(projectName) + ".internal"
}
