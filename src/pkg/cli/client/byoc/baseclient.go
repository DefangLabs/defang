package byoc

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type ErrMultipleProjects struct {
	ProjectNames []string
}

func (mp ErrMultipleProjects) Error() string {
	return fmt.Sprintf("multiple projects found; use the --project-name flag to specify a project: \n%v", strings.Join(mp.ProjectNames, "\n"))
}

type ProjectBackend interface {
	BootstrapList(context.Context, bool) (iter.Seq[string], error)
	GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error)
	GetPrivateDomain(projectName string) string
}

type ServiceInfoUpdater interface {
	UpdateServiceInfo(ctx context.Context, serviceInfo *defangv1.ServiceInfo, projectName, delegateDomain string, service composeTypes.ServiceConfig) error
}

type CanIUseConfig struct {
	AllowGPU      bool
	AllowScaling  bool
	CDImage       string
	PulumiVersion string
}

type ByocBaseClient struct {
	client.RetryDelayer

	Prefix      string
	PulumiStack string
	SetupDone   bool
	TenantLabel types.TenantLabel
	CanIUseConfig

	projectBackend ProjectBackend
}

func NewByocBaseClient(tenantLabel types.TenantLabel, backend ProjectBackend, stack string) *ByocBaseClient {
	if stack == "" {
		stack = stacks.DefaultBeta // backwards compat
	}
	return &ByocBaseClient{
		Prefix:         pkg.Getenv("DEFANG_PREFIX", "Defang"), // prefix for all resources created by Defang
		PulumiStack:    pkg.Getenv("DEFANG_SUFFIX", stack),
		TenantLabel:    tenantLabel,
		projectBackend: backend,
	}
}

func (b *ByocBaseClient) GetStackName() string {
	return b.PulumiStack
}

func (b *ByocBaseClient) Debug(context.Context, *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return nil, client.ErrNotImplemented("AI debugging is not yet supported for BYOC")
}

func (b *ByocBaseClient) SetCanIUseConfig(quotas *defangv1.CanIUseResponse) {
	b.CanIUseConfig.AllowGPU = quotas.Gpu
	b.CanIUseConfig.AllowScaling = quotas.AllowScaling
	b.CanIUseConfig.CDImage = quotas.CdImage
	b.CanIUseConfig.PulumiVersion = quotas.PulumiVersion
}

// getServiceLabel returns a DNS-safe label for the given service
func getServiceLabel(serviceName string) string {
	// Technically DNS names can have underscores, but these are reserved for SRV records and some systems have issues with them.
	return dns.SafeLabel(strings.ReplaceAll(serviceName, "_", "-"))
}

func (b *ByocBaseClient) ServicePrivateDNS(serviceName string) string {
	// Private services can be accessed by just using the (sanitized) service name
	return getServiceLabel(serviceName)
}

func (b *ByocBaseClient) RemoteProjectName(ctx context.Context) (string, error) {
	// Get the list of projects from remote
	stacks, err := b.projectBackend.BootstrapList(ctx, false)
	if err != nil {
		return "", fmt.Errorf("no cloud projects found: %w", err)
	}
	var projectNames []string
	for name := range stacks {
		projectName := strings.Split(name, "/")[0] // Remove the stack name
		projectNames = append(projectNames, projectName)
	}

	if len(projectNames) == 0 {
		return "", errors.New("no cloud projects found")
	}

	if len(projectNames) > 1 {
		return "", ErrMultipleProjects{ProjectNames: projectNames}
	}
	term.Debug("Using default project:", projectNames[0])
	return projectNames[0], nil
}

type ErrNoPermission string

func (e ErrNoPermission) Error() string {
	return "current subscription tier does not allow this action: " + string(e)
}

func (b *ByocBaseClient) GetServiceInfos(ctx context.Context, projectName, delegateDomain, etag string, services map[string]composeTypes.ServiceConfig) ([]*defangv1.ServiceInfo, error) {
	numGPUS := compose.GetNumOfGPUs(services)
	if numGPUS > 0 && !b.AllowGPU {
		return nil, ErrNoPermission("usage of GPUs. Please upgrade on https://s.defang.io/subscription")
	}

	serviceInfoMap := make(map[string]*Node)
	for _, service := range services {
		serviceInfo, err := b.update(ctx, projectName, delegateDomain, service)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", service.Name, err)
		}
		serviceInfo.Etag = etag // same etag for all services
		serviceInfoMap[service.Name] = &Node{
			Name:        service.Name,
			Deps:        service.GetDependencies(),
			ServiceInfo: serviceInfo,
		}
	}

	// Reorder the serviceInfos to make sure the dependencies are created first
	serviceInfos := topologicalSort(serviceInfoMap)

	// Ensure all service endpoints are unique
	endpoints := make(map[string]bool)
	for _, serviceInfo := range serviceInfos {
		for _, endpoint := range serviceInfo.Endpoints {
			if endpoints[endpoint] {
				return nil, fmt.Errorf("duplicate endpoint: %s", endpoint) // CodeInvalidArgument
			}
			endpoints[endpoint] = true
		}
	}

	return serviceInfos, nil
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
		if node == nil || node.Visited {
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

// This function was based on update function from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) update(ctx context.Context, projectName, delegateDomain string, service composeTypes.ServiceConfig) (*defangv1.ServiceInfo, error) {
	if err := compose.ValidateService(&service); err != nil {
		return nil, err // caller prepends service name
	}

	pkg.Ensure(projectName != "", "ProjectName not set")
	healthCheckPath, _ := compose.GetHealthCheckPathAndPort(service.HealthCheck)
	si := &defangv1.ServiceInfo{
		AllowScaling:    b.AllowScaling,
		Domainname:      service.DomainName,
		Etag:            types.NewEtag(), // TODO: could be hash for dedup/idempotency
		Project:         projectName,     // was: tenant
		Service:         &defangv1.Service{Name: service.Name},
		HealthcheckPath: healthCheckPath,
	}

	hasHost := false
	hasIngress := false
	if _, ok := service.Extensions["x-defang-static-files"]; !ok {
		for _, port := range service.Ports {
			hasIngress = hasIngress || port.Mode == compose.Mode_INGRESS
			hasHost = hasHost || port.Mode == compose.Mode_HOST
			si.Endpoints = append(si.Endpoints, b.GetEndpoint(service.Name, projectName, delegateDomain, &port))
		}
	} else {
		si.PublicFqdn = b.GetPublicFqdn(projectName, delegateDomain, service.Name)
		si.Endpoints = append(si.Endpoints, si.PublicFqdn)
	}
	if hasIngress {
		// si.LbIps = b.PrivateLbIps // only set LB IPs if there are ingress ports // FIXME: double check this is not being used at all
		si.PublicFqdn = b.GetPublicFqdn(projectName, delegateDomain, service.Name)
	}
	if hasHost { // TODO: this should be network based instead of host vs ingress
		si.PrivateFqdn = b.GetPrivateFqdn(projectName, service.Name)
	}

	si.Status = "UPDATE_QUEUED"
	si.State = defangv1.ServiceState_UPDATE_QUEUED
	if service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
		si.State = defangv1.ServiceState_BUILD_QUEUED
	}

	if siUpdater, ok := b.projectBackend.(ServiceInfoUpdater); ok {
		if err := siUpdater.UpdateServiceInfo(ctx, si, projectName, delegateDomain, service); err != nil {
			return nil, err // caller prepends service name
		}
	}

	return si, nil
}

// stackDir returns a stack-qualified path, like the Pulumi TS function `stackDir`
func (b *ByocBaseClient) StackDir(projectName, name string) string {
	pkg.Ensure(projectName != "", "ProjectName not set")
	prefix := []string{""} // for leading slash
	if b.Prefix != "" {
		prefix = []string{"", b.Prefix}
	}
	return strings.Join(append(prefix, projectName, b.PulumiStack, name), "/") // same as shared/common.ts
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) GetEndpoint(serviceName string, projectName, delegateDomain string, port *composeTypes.ServicePortConfig) string {
	if port.Mode == compose.Mode_HOST {
		privateFqdn := b.GetPrivateFqdn(projectName, serviceName)
		return fmt.Sprintf("%s:%d", privateFqdn, port.Target)
	}
	if projectName == "" {
		return ":443" // placeholder for the public ALB/distribution
	}
	return fmt.Sprintf("%s--%d.%s", getServiceLabel(serviceName), port.Target, delegateDomain) // delegateDomain already contains project/stack
}

func (*ByocBaseClient) UpdateShardDomain(ctx context.Context) error {
	// BYOC providers manage their own domains and don't use shard domains
	return nil
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) GetPublicFqdn(projectName, delegateDomain, serviceName string) string {
	if projectName == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", getServiceLabel(serviceName), delegateDomain) // delegateDomain already contains project/stack
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocBaseClient) GetPrivateFqdn(projectName string, serviceName string) string {
	return fmt.Sprintf("%s.%s", b.ServicePrivateDNS(serviceName), b.projectBackend.GetPrivateDomain(projectName))
}

func (b *ByocBaseClient) GetProjectUpdatePath(projectName string) string {
	// Path to the state file, Defined at: https://github.com/DefangLabs/defang-mvp/blob/main/pulumi/cd/aws/byoc.ts#L104
	pkg.Ensure(projectName != "", "ProjectName not set")
	return fmt.Sprintf("projects/%s/%s/project.pb", projectName, b.PulumiStack)
}

func (b *ByocBaseClient) ServicePublicDNS(serviceName string, projectName string) string {
	tenantLabel := dns.SafeLabel(string(b.TenantLabel))
	// TODO: this should use the delegate domain we got from Fabric
	return fmt.Sprintf("%s.%s.%s.defang.app", getServiceLabel(serviceName), b.GetProjectLabel(projectName), tenantLabel)
}

func (b *ByocBaseClient) GetStackNameForDomain() string {
	// Projects which were deployed before stacks were introduced, were
	// deployed with the implicit stack name "beta", but this stack name was
	// excluded from the delegate subdomain. Now that stacks are explicit,
	// and we want them to appear in the delegate, we need to preserve
	// backwards compatibility with stacks named "beta". This backwards-
	// compatibility is implemented here by returning a Stack name of "" in
	// place of "beta", so that Fabric will treat these stacks as if there
	// was no explicit stack.
	if b.PulumiStack == stacks.DefaultBeta {
		return ""
	}
	return b.PulumiStack
}

func (b *ByocBaseClient) GetStackName() string {
	return b.PulumiStack
}

// GetProjectLabel returns the DNS-safe project label, including stack (if applicable)
func (b *ByocBaseClient) GetProjectLabel(projectName string) string {
	if stack := b.GetStackNameForDomain(); stack != "" {
		projectName = projectName + "-" + stack
	}
	return dns.SafeLabel(projectName)
}
