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
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

// This function was copied from Fabric controller and slightly modified to work with BYOC
func DnsSafeLabel(fqn string) string {
	return strings.ReplaceAll(DnsSafe(fqn), ".", "-")
}

func DnsSafe(fqdn string) string {
	return strings.ReplaceAll(strings.ToLower(fqdn), "_", "-")
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

type ServiceInfoUpdater interface {
	UpdateServiceInfo(ctx context.Context, serviceInfo *defangv1.ServiceInfo, projectName, delegateDomain string, service composeTypes.ServiceConfig) error
}

type HasStackSupport interface {
	GetStackName() string
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
	domain := projectLabel + "." + DnsSafe(zone)
	if hasStack, ok := b.projectBackend.(HasStackSupport); ok {
		domain = hasStack.GetStackName() + "." + domain
	}
	return domain
}

func (b *ByocBaseClient) GetServiceInfos(ctx context.Context, projectName, delegateDomain, etag string, services map[string]composeTypes.ServiceConfig) ([]*defangv1.ServiceInfo, error) {
	serviceInfoMap := make(map[string]*Node)
	for _, service := range services {
		serviceInfo, err := b.update(ctx, projectName, delegateDomain, service)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", service.Name, err)
		}
		serviceInfo.Etag = etag // same etag for all services
		serviceInfoMap[service.Name] = &Node{
			Name:        service.Name,
			Deps:        getDependencies(service),
			ServiceInfo: serviceInfo,
		}
	}

	// Reorder the serviceInfos to make sure the dependencies are created first
	return topologicalSort(serviceInfoMap), nil
}

// Simple DFS topological sort to make sure the dependencies are created first
type Node struct {
	Name        string
	Deps        []string
	ServiceInfo *defangv1.ServiceInfo
	Visited     bool
}

func topologicalSort(nodes map[string]*Node) []*defangv1.ServiceInfo {
	serviceInfos := make([]*defangv1.ServiceInfo, 0, len(nodes))

	var visit func(node *Node)
	visit = func(node *Node) {
		if node.Visited {
			return
		}
		node.Visited = true

		for _, dep := range node.Deps {
			visit(nodes[dep])
		}

		serviceInfos = append(serviceInfos, node.ServiceInfo)
	}

	for _, node := range nodes {
		visit(node)
	}
	return serviceInfos
}

func getDependencies(service composeTypes.ServiceConfig) []string {
	deps := []string{}
	for depServiceName := range service.DependsOn {
		deps = append(deps, depServiceName)
	}
	return deps
}

// This function was based on update function from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) update(ctx context.Context, projectName, delegateDomain string, service composeTypes.ServiceConfig) (*defangv1.ServiceInfo, error) {
	if err := compose.ValidateService(&service); err != nil {
		return nil, err
	}

	pkg.Ensure(projectName != "", "ProjectName not set")
	si := &defangv1.ServiceInfo{
		Etag:    pkg.RandomID(), // TODO: could be hash for dedup/idempotency
		Project: projectName,    // was: tenant
		Service: &defangv1.Service{Name: service.Name},
	}

	hasHost := false
	hasIngress := false
	fqn := service.Name
	if _, ok := service.Extensions["x-defang-static-files"]; !ok {
		for _, port := range service.Ports {
			hasIngress = hasIngress || port.Mode == compose.Mode_INGRESS
			hasHost = hasHost || port.Mode == compose.Mode_HOST
			si.Endpoints = append(si.Endpoints, b.GetEndpoint(fqn, projectName, delegateDomain, &port))
			mode := defangv1.Mode_INGRESS
			if port.Mode == compose.Mode_HOST {
				mode = defangv1.Mode_HOST
			}
			si.Service.Ports = append(si.Service.Ports, &defangv1.Port{
				Target: port.Target,
				Mode:   mode,
			})
		}
	} else {
		si.PublicFqdn = b.GetPublicFqdn(projectName, delegateDomain, fqn)
		si.Endpoints = append(si.Endpoints, si.PublicFqdn)
	}
	if hasIngress {
		// si.LbIps = b.PrivateLbIps // only set LB IPs if there are ingress ports // FIXME: double check this is not being used at all
		si.PublicFqdn = b.GetPublicFqdn(projectName, delegateDomain, fqn)
	}
	if hasHost {
		si.PrivateFqdn = b.GetPrivateFqdn(projectName, fqn)
	}

	if service.DomainName != "" {
		if !hasIngress && service.Extensions["x-defang-static-files"] == nil {
			return nil, errors.New("domainname requires at least one ingress port") // retryable CodeFailedPrecondition
		}
	}

	si.Status = "UPDATE_QUEUED"
	si.State = defangv1.ServiceState_UPDATE_QUEUED
	if service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
		si.State = defangv1.ServiceState_BUILD_QUEUED
	}

	if siUpdater, ok := b.projectBackend.(ServiceInfoUpdater); ok {
		if err := siUpdater.UpdateServiceInfo(ctx, si, projectName, delegateDomain, service); err != nil {
			return nil, err
		}
	}

	return si, nil
}

// stackDir returns a stack-qualified path, like the Pulumi TS function `stackDir`
func (b *ByocBaseClient) StackDir(projectName, name string) string {
	pkg.Ensure(projectName != "", "ProjectName not set")
	return fmt.Sprintf("/%s/%s/%s/%s", DefangPrefix, projectName, b.PulumiStack, name) // same as shared/common.ts
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) GetEndpoint(fqn string, projectName, delegateDomain string, port *composeTypes.ServicePortConfig) string {
	if port.Mode == compose.Mode_HOST {
		privateFqdn := b.GetPrivateFqdn(projectName, fqn)
		return fmt.Sprintf("%s:%d", privateFqdn, port.Target)
	}
	projectDomain := b.GetProjectDomain(projectName, delegateDomain)
	if projectDomain == "" {
		return ":443" // placeholder for the public ALB/distribution
	}
	safeFqn := DnsSafeLabel(fqn)
	return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, projectDomain)
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) GetPublicFqdn(projectName, delegateDomain, fqn string) string {
	if projectName == "" {
		return "" //b.fqdn
	}
	safeFqn := DnsSafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.GetProjectDomain(projectName, delegateDomain))
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b ByocBaseClient) GetPrivateFqdn(projectName string, fqn string) string {
	safeFqn := DnsSafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, GetPrivateDomain(projectName)) // TODO: consider merging this with ServiceDNS
}

func GetPrivateDomain(projectName string) string {
	return DnsSafeLabel(projectName) + ".internal"
}
