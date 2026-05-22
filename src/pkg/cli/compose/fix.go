package compose

import (
	"fmt"
	"sort"
	"strings"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

const defaultRestartPolicy = "unless-stopped"

type FixResult struct {
	Service string
	Field   string
	Action  string // "added", "removed", "changed"
	Before  string
	After   string
	Reason  string
}

func FixProject(project *Project) []FixResult {
	if project == nil {
		return nil
	}

	var results []FixResult
	for _, name := range sortedServiceNames(project) {
		service := project.Services[name]
		results = append(results, fixService(&service)...)
		project.Services[name] = service
	}
	return results
}

func sortedServiceNames(project *Project) []string {
	names := make([]string, 0, len(project.Services))
	for name := range project.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fixService(service *composeTypes.ServiceConfig) []FixResult {
	var results []FixResult
	repo := GetImageRepo(service.Image)
	isManagedStoreImage := IsPostgresRepo(repo) || IsRedisRepo(repo) || IsMongoRepo(repo)

	results = append(results, fixPorts(service, isManagedStoreImage)...)
	results = append(results, fixManagedServiceExtensions(service, repo)...)
	results = append(results, fixResources(service)...)
	results = append(results, fixRestart(service)...)
	results = append(results, fixHostname(service)...)
	results = append(results, fixUnsupportedDirectives(service)...)
	results = append(results, fixIngressHealthcheck(service)...)

	return results
}

func fixPorts(service *composeTypes.ServiceConfig, isManagedStoreImage bool) []FixResult {
	var results []FixResult
	for i := range service.Ports {
		port := &service.Ports[i]
		if port.Mode != "" {
			continue
		}

		mode := Mode_INGRESS
		reason := ""
		if port.Protocol == Protocol_UDP {
			mode = Mode_HOST
			reason = "UDP port"
		} else if isManagedStoreImage {
			mode = Mode_HOST
			reason = "database image"
		}
		port.Mode = mode
		results = append(results, FixResult{
			Service: service.Name,
			Field:   "mode",
			Action:  "added",
			After:   mode,
			Reason:  portReason(port.Target, reason),
		})
	}
	return results
}

func portReason(target uint32, reason string) string {
	if reason == "" {
		return fmt.Sprintf("port %d", target)
	}
	return fmt.Sprintf("port %d (%s)", target, reason)
}

func fixManagedServiceExtensions(service *composeTypes.ServiceConfig, repo string) []FixResult {
	switch {
	case IsPostgresRepo(repo):
		return addManagedServiceExtension(service, "x-defang-postgres", "postgres image detected")
	case IsRedisRepo(repo):
		return addManagedServiceExtension(service, "x-defang-redis", "redis image detected")
	case IsMongoRepo(repo):
		return addManagedServiceExtension(service, "x-defang-mongodb", "mongo image detected")
	default:
		return nil
	}
}

func addManagedServiceExtension(service *composeTypes.ServiceConfig, key, reason string) []FixResult {
	if service.Extensions == nil {
		service.Extensions = composeTypes.Extensions{}
	}
	if _, ok := service.Extensions[key]; ok {
		return nil
	}
	service.Extensions[key] = true
	return []FixResult{{
		Service: service.Name,
		Field:   key,
		Action:  "added",
		After:   "true",
		Reason:  reason,
	}}
}

func fixResources(service *composeTypes.ServiceConfig) []FixResult {
	var results []FixResult
	if service.Deploy == nil {
		service.Deploy = &composeTypes.DeployConfig{}
	}
	if service.Deploy.Resources.Limits != nil && service.Deploy.Resources.Reservations == nil {
		limits := *service.Deploy.Resources.Limits
		service.Deploy.Resources.Reservations = &limits
		results = append(results, FixResult{
			Service: service.Name,
			Field:   "deploy.resources.reservations",
			Action:  "added",
			After:   "deploy.resources.limits",
			Reason:  "limits used as reservations",
		})
	}
	if service.Deploy.Resources.Reservations == nil {
		service.Deploy.Resources.Reservations = &composeTypes.Resource{}
	}
	if service.Deploy.Resources.Reservations.MemoryBytes == 0 {
		memory := composeTypes.UnitBytes(512 * MiB)
		after := "512M"
		if service.Extensions["x-defang-static-files"] != nil {
			memory = composeTypes.UnitBytes(256 * MiB)
			after = "256M"
		}
		service.Deploy.Resources.Reservations.MemoryBytes = memory
		results = append(results, FixResult{
			Service: service.Name,
			Field:   "deploy.resources.reservations.memory",
			Action:  "added",
			After:   after,
			Reason:  "missing memory reservation",
		})
	}
	return results
}

func fixRestart(service *composeTypes.ServiceConfig) []FixResult {
	restart := restartFromDeployPolicy(service)
	if restart == "" && isSupportedRestart(service.Restart) {
		return nil
	}

	before := service.Restart
	if restart == "" {
		restart = defaultRestartPolicy
	}
	service.Restart = restart

	reason := "unsupported restart policy"
	if service.Deploy != nil && service.Deploy.RestartPolicy != nil {
		reason = "deploy.restart_policy is unsupported"
	} else if before == "" {
		reason = "missing restart policy"
	}

	if service.Deploy != nil && service.Deploy.RestartPolicy != nil {
		service.Deploy.RestartPolicy = nil
	}

	action := "changed"
	if before == "" {
		action = "added"
	}
	return []FixResult{{
		Service: service.Name,
		Field:   "restart",
		Action:  action,
		Before:  before,
		After:   restart,
		Reason:  reason,
	}}
}

func restartFromDeployPolicy(service *composeTypes.ServiceConfig) string {
	if service.Deploy == nil || service.Deploy.RestartPolicy == nil {
		return ""
	}
	switch service.Deploy.RestartPolicy.Condition {
	case "", "any":
		return "always"
	default:
		return defaultRestartPolicy
	}
}

func isSupportedRestart(restart string) bool {
	return restart == "always" || restart == defaultRestartPolicy
}

func fixHostname(service *composeTypes.ServiceConfig) []FixResult {
	if service.Hostname == "" {
		return nil
	}
	before := service.Hostname
	service.DomainName = service.Hostname
	service.Hostname = ""
	return []FixResult{{
		Service: service.Name,
		Field:   "domainname",
		Action:  "changed",
		Before:  before,
		After:   service.DomainName,
		Reason:  "hostname is unsupported",
	}}
}

func fixUnsupportedDirectives(service *composeTypes.ServiceConfig) []FixResult {
	var results []FixResult
	if len(service.DNS) != 0 {
		service.DNS = nil
		results = append(results, removedDirective(service.Name, "dns"))
	}
	if len(service.DNSSearch) != 0 {
		service.DNSSearch = nil
		results = append(results, removedDirective(service.Name, "dns_search"))
	}
	if len(service.Devices) != 0 {
		service.Devices = nil
		results = append(results, removedDirective(service.Name, "devices"))
	}
	if len(service.DeviceCgroupRules) != 0 {
		service.DeviceCgroupRules = nil
		results = append(results, removedDirective(service.Name, "device_cgroup_rules"))
	}
	if len(service.GroupAdd) != 0 {
		service.GroupAdd = nil
		results = append(results, removedDirective(service.Name, "group_add"))
	}
	return results
}

func removedDirective(service, field string) FixResult {
	return FixResult{
		Service: service,
		Field:   field,
		Action:  "removed",
		Before:  "present",
		Reason:  "unsupported directive",
	}
}

func fixIngressHealthcheck(service *composeTypes.ServiceConfig) []FixResult {
	if service.HealthCheck != nil && !service.HealthCheck.Disable {
		return nil
	}
	for _, port := range service.Ports {
		if port.Mode != Mode_INGRESS {
			continue
		}
		url := fmt.Sprintf("http://localhost:%d/", port.Target)
		service.HealthCheck = &composeTypes.HealthCheckConfig{
			Test: composeTypes.HealthCheckTest{"CMD", "curl", "-f", url},
		}
		return []FixResult{{
			Service: service.Name,
			Field:   "healthcheck",
			Action:  "added",
			After:   strings.Join(service.HealthCheck.Test, " "),
			Reason:  fmt.Sprintf("ingress port %d", port.Target),
		}}
	}
	return nil
}
