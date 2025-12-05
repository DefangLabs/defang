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
}

type ServiceInfoUpdater interface {
	UpdateServiceInfo(ctx context.Context, serviceInfo *defangv1.ServiceInfo, projectName, delegateDomain string, service composeTypes.ServiceConfig) error
}

type HasStackSupport interface {
	GetStackName() string
}

type CanIUseConfig struct {
	AllowGPU      bool
	AllowScaling  bool
	CDImage       string
	PulumiVersion string
}

type ByocBaseClient struct {
	client.RetryDelayer

	Prefix                  string
	PulumiStack             string
	SetupDone               bool
	ShouldDelegateSubdomain bool
	TenantName              string
	CanIUseConfig

	projectBackend ProjectBackend
}

func NewByocBaseClient(tenantName types.TenantName, backend ProjectBackend, stack string) *ByocBaseClient {
	if stack == "" {
		stack = "beta" // backwards compat
	}
	b := &ByocBaseClient{
		Prefix:         pkg.Getenv("DEFANG_PREFIX", "Defang"), // prefix for all resources created by Defang
		TenantName:     string(tenantName),
		PulumiStack:    pkg.Getenv("DEFANG_SUFFIX", stack),
		projectBackend: backend,
	}
	return b
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

func (b *ByocBaseClient) ServicePrivateDNS(name string) string {
	return dns.SafeLabel(name) // TODO: consider merging this with getPrivateFqdn
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

func (b *ByocBaseClient) GetProjectDomain(projectName, zone string) string {
	if projectName == "" {
		return "" // no project name => no custom domain
	}
	domain := dns.Normalize(zone)
	if hasStack, ok := b.projectBackend.(HasStackSupport); ok {
		domain = hasStack.GetStackName() + "." + domain
	}
	return domain
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
	fqn := strings.ReplaceAll(service.Name, "_", "-")
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
	if hasHost { // TODO: this should be network based instead of host vs ingress
		si.PrivateFqdn = b.GetPrivateFqdn(projectName, fqn)
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
func (b *ByocBaseClient) GetEndpoint(fqn string, projectName, delegateDomain string, port *composeTypes.ServicePortConfig) string {
	if port.Mode == compose.Mode_HOST {
		privateFqdn := b.GetPrivateFqdn(projectName, fqn)
		return fmt.Sprintf("%s:%d", privateFqdn, port.Target)
	}
	projectDomain := b.GetProjectDomain(projectName, delegateDomain)
	if projectDomain == "" {
		return ":443" // placeholder for the public ALB/distribution
	}
	safeFqn := dns.SafeLabel(fqn)
	return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, projectDomain)
}

func (b *ByocBaseClient) UpdateShardDomain(ctx context.Context) error {
	// BYOC providers manage their own domains and don't use shard domains
	return nil
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b ByocBaseClient) GetPublicFqdn(projectName, delegateDomain, fqn string) string {
	if projectName == "" {
		return "" //b.fqdn
	}
	safeFqn := dns.SafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.GetProjectDomain(projectName, delegateDomain))
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b ByocBaseClient) GetPrivateFqdn(projectName string, fqn string) string {
	safeFqn := dns.SafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, GetPrivateDomain(projectName)) // TODO: consider merging this with ServicePrivateDNS
}
