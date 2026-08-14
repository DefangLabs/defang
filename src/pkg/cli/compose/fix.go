package compose

import (
	"fmt"
	"sort"

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
	results = append(results, fixLimitsToReservations(service)...)
	results = append(results, fixRestart(service)...)
	results = append(results, fixUnsupportedDirectives(service)...)

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

func fixLimitsToReservations(service *composeTypes.ServiceConfig) []FixResult {
	if service.Deploy == nil {
		return nil
	}
	if service.Deploy.Resources.Limits == nil || service.Deploy.Resources.Reservations != nil {
		return nil
	}
	limits := *service.Deploy.Resources.Limits
	service.Deploy.Resources.Reservations = &limits
	return []FixResult{{
		Service: service.Name,
		Field:   "deploy.resources.reservations",
		Action:  "added",
		After:   "copied from deploy.resources.limits",
		Reason:  "Defang uses reservations for scheduling, not limits",
	}}
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
		reason = "deploy.restart_policy is unsupported; converted to service-level restart"
		service.Deploy.RestartPolicy = nil
	} else if before == "" {
		reason = "missing restart policy"
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
