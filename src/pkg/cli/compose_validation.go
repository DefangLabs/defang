package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func validateProject(project *compose.Project) error {
	if project == nil {
		return errors.New("no project found")
	}
	for _, svccfg := range project.Services {
		normalized := NormalizeServiceName(svccfg.Name)
		if !pkg.IsValidServiceName(normalized) {
			// FIXME: this is too strict; we should allow more characters like underscores and dots
			return fmt.Errorf("service name is invalid: %q", svccfg.Name)
		}
		if normalized != svccfg.Name {
			warnf("service name %q was normalized to %q", svccfg.Name, normalized)
		}
		if svccfg.ReadOnly {
			warnf("unsupported compose directive: read_only")
		}
		if svccfg.Restart == "" {
			warnf("missing compose directive: `restart`; assuming 'unless-stopped' (add 'restart' to silence)")
		} else if svccfg.Restart != "always" && svccfg.Restart != "unless-stopped" {
			warnf("unsupported compose directive: restart; assuming 'unless-stopped' (add 'restart' to silence)")
		}
		if svccfg.ContainerName != "" {
			warnf("unsupported compose directive: container_name")
		}
		if svccfg.Hostname != "" {
			return fmt.Errorf("unsupported compose directive: hostname; consider using 'domainname' instead")
		}
		if len(svccfg.DNSSearch) != 0 {
			return fmt.Errorf("unsupported compose directive: dns_search")
		}
		if len(svccfg.DNSOpts) != 0 {
			warnf("unsupported compose directive: dns_opt")
		}
		if len(svccfg.DNS) != 0 {
			return fmt.Errorf("unsupported compose directive: dns")
		}
		if len(svccfg.Devices) != 0 {
			return fmt.Errorf("unsupported compose directive: devices")
		}
		if len(svccfg.DependsOn) != 0 {
			warnf("unsupported compose directive: depends_on")
		}
		if len(svccfg.DeviceCgroupRules) != 0 {
			return fmt.Errorf("unsupported compose directive: device_cgroup_rules")
		}
		if len(svccfg.Entrypoint) > 0 {
			return fmt.Errorf("unsupported compose directive: entrypoint")
		}
		if len(svccfg.GroupAdd) > 0 {
			return fmt.Errorf("unsupported compose directive: group_add")
		}
		if len(svccfg.Ipc) > 0 {
			warnf("unsupported compose directive: ipc")
		}
		if len(svccfg.Uts) > 0 {
			warnf("unsupported compose directive: uts")
		}
		if svccfg.Isolation != "" {
			warnf("unsupported compose directive: isolation")
		}
		if svccfg.MacAddress != "" {
			warnf("unsupported compose directive: mac_address")
		}
		if len(svccfg.Labels) > 0 {
			warnf("unsupported compose directive: labels") // TODO: add support for labels
		}
		if len(svccfg.Links) > 0 {
			warnf("unsupported compose directive: links")
		}
		if svccfg.Logging != nil {
			warnf("unsupported compose directive: logging")
		}
		for name := range svccfg.Networks {
			if _, ok := project.Networks[name]; !ok {
				warnf("network %v used by service %v is not defined in the top-level networks section", name, svccfg.Name)
			}
		}
		if len(svccfg.Volumes) > 0 {
			warnf("unsupported compose directive: volumes") // TODO: add support for volumes
		}
		if len(svccfg.VolumesFrom) > 0 {
			warnf("unsupported compose directive: volumes_from") // TODO: add support for volumes_from
		}
		if svccfg.Build != nil {
			if svccfg.Build.Dockerfile != "" {
				if filepath.IsAbs(svccfg.Build.Dockerfile) {
					return fmt.Errorf("dockerfile path must be relative to the build context: %q", svccfg.Build.Dockerfile)
				}
				if strings.HasPrefix(svccfg.Build.Dockerfile, "../") {
					return fmt.Errorf("dockerfile path must be inside the build context: %q", svccfg.Build.Dockerfile)
				}
				// Check if the dockerfile exists
				dockerfilePath := filepath.Join(svccfg.Build.Context, svccfg.Build.Dockerfile)
				if _, err := os.Stat(dockerfilePath); err != nil {
					return fmt.Errorf("dockerfile not found: %q", dockerfilePath)
				}
			}
			if svccfg.Build.SSH != nil {
				return fmt.Errorf("unsupported compose directive: build ssh")
			}
			if len(svccfg.Build.Labels) != 0 {
				warnf("unsupported compose directive: build labels") // TODO: add support for Kaniko --label
			}
			if len(svccfg.Build.CacheFrom) != 0 {
				warnf("unsupported compose directive: build cache_from")
			}
			if len(svccfg.Build.CacheTo) != 0 {
				warnf("unsupported compose directive: build cache_to")
			}
			if svccfg.Build.NoCache {
				warnf("unsupported compose directive: build no_cache")
			}
			if len(svccfg.Build.ExtraHosts) != 0 {
				return fmt.Errorf("unsupported compose directive: build extra_hosts")
			}
			if svccfg.Build.Isolation != "" {
				warnf("unsupported compose directive: build isolation")
			}
			if svccfg.Build.Network != "" {
				return fmt.Errorf("unsupported compose directive: build network")
			}
			if len(svccfg.Build.Secrets) != 0 {
				return fmt.Errorf("unsupported compose directive: build secrets") // TODO: support build secrets
			}
			if len(svccfg.Build.Tags) != 0 {
				return fmt.Errorf("unsupported compose directive: build tags")
			}
			if len(svccfg.Build.Platforms) != 0 {
				return fmt.Errorf("unsupported compose directive: build platforms")
			}
			if svccfg.Build.Privileged {
				return fmt.Errorf("unsupported compose directive: build privileged")
			}
			if svccfg.Build.DockerfileInline != "" {
				return fmt.Errorf("unsupported compose directive: build dockerfile_inline")
			}
		}
		for _, secret := range svccfg.Secrets {
			if !pkg.IsValidSecretName(secret.Source) {
				return fmt.Errorf("secret name is invalid: %q", secret.Source)
			}
			if secret.Target != "" {
				return fmt.Errorf("unsupported compose directive: secret target")
			}
			if s, ok := project.Secrets[secret.Source]; !ok {
				warnf("secret %q is not defined in the top-level secrets section", secret.Source)
			} else if s.Name != "" && s.Name != secret.Source {
				return fmt.Errorf("unsupported secret %q: cannot override name %q", secret.Source, s.Name) // TODO: support custom secret names
			} else if !s.External {
				warnf("unsupported secret %q: not marked external:true", secret.Source) // TODO: support secrets from environment/file
			}
		}
		err := validatePorts(svccfg.Ports)
		if err != nil {
			return err
		}
		if svccfg.HealthCheck == nil || svccfg.HealthCheck.Disable {
			// Show a warning when we have ingress ports but no explicit healthcheck
			for _, port := range svccfg.Ports {
				if port.Mode == "ingress" {
					warnf("ingress port without healthcheck defaults to GET / HTTP/1.1")
					break
				}
			}
		} else {
			timeout := 30 // default per compose spec
			if svccfg.HealthCheck.Timeout != nil {
				if *svccfg.HealthCheck.Timeout%1e9 != 0 {
					warnf("healthcheck timeout must be a multiple of 1s")
				}
				timeout = int(*svccfg.HealthCheck.Timeout / 1e9)
			}
			interval := 30 // default per compose spec
			if svccfg.HealthCheck.Interval != nil {
				if *svccfg.HealthCheck.Interval%1e9 != 0 {
					warnf("healthcheck interval must be a multiple of 1s")
				}
				interval = int(*svccfg.HealthCheck.Interval / 1e9)
			}
			// Technically this should test for <= but both interval and timeout have 30s as the default value
			if interval < timeout || timeout <= 0 {
				return fmt.Errorf("healthcheck timeout %ds must be positive and smaller than the interval %ds", timeout, interval)
			}
			if svccfg.HealthCheck.StartPeriod != nil {
				warnf("unsupported compose directive: healthcheck start_period")
			}
			if svccfg.HealthCheck.StartInterval != nil {
				warnf("unsupported compose directive: healthcheck start_interval")
			}
		}
		if svccfg.Deploy != nil {
			if svccfg.Deploy.Mode != "" && svccfg.Deploy.Mode != "replicated" {
				return fmt.Errorf("unsupported compose directive: deploy mode: %q", svccfg.Deploy.Mode)
			}
			if len(svccfg.Deploy.Labels) > 0 {
				warnf("unsupported compose directive: deploy labels")
			}
			if svccfg.Deploy.UpdateConfig != nil {
				return fmt.Errorf("unsupported compose directive: deploy update_config")
			}
			if svccfg.Deploy.RollbackConfig != nil {
				return fmt.Errorf("unsupported compose directive: deploy rollback_config")
			}
			if svccfg.Deploy.RestartPolicy != nil {
				return fmt.Errorf("unsupported compose directive: deploy restart_policy")
			}
			if len(svccfg.Deploy.Placement.Constraints) != 0 || len(svccfg.Deploy.Placement.Preferences) != 0 || svccfg.Deploy.Placement.MaxReplicas != 0 {
				warnf("unsupported compose directive: deploy placement")
			}
			if svccfg.Deploy.EndpointMode != "" {
				return fmt.Errorf("unsupported compose directive: deploy endpoint_mode")
			}
			if svccfg.Deploy.Resources.Limits != nil && svccfg.Deploy.Resources.Reservations == nil {
				warnf("no reservations specified; using limits as reservations")
			}
			reservations := getResourceReservations(svccfg.Deploy.Resources)
			if reservations != nil && reservations.NanoCPUs != "" {
				cpus, err := strconv.ParseFloat(reservations.NanoCPUs, 32)
				if err != nil || cpus < 0 { // "0" just means "as small as possible"
					return fmt.Errorf("invalid value for cpus: %q", reservations.NanoCPUs)
				}
			}
		}
		var reservations *compose.Resource
		if svccfg.Deploy != nil {
			reservations = getResourceReservations(svccfg.Deploy.Resources)
		}

		if svccfg.Deploy == nil || reservations == nil || reservations.MemoryBytes == 0 {
			warnf("missing memory reservation; specify deploy.resources.reservations.memory to avoid out-of-memory errors")
		}

		if dnsRoleVal := svccfg.Extensions["x-defang-dns-role"]; dnsRoleVal != nil {
			if _, ok := dnsRoleVal.(string); !ok {
				return fmt.Errorf("x-defang-dns-role must be a string")
			}
		}

		if staticFilesVal := svccfg.Extensions["x-defang-static-files"]; staticFilesVal != nil {
			if _, ok := staticFilesVal.(string); !ok {
				return fmt.Errorf("x-defang-static-files must be a string")
			}
		}
	}
	return nil
}
