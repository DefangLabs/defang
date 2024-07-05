package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/sirupsen/logrus"
)

func ValidateProject(project *compose.Project) error {
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
			warnf("service %q: unsupported compose directive: read_only", svccfg.Name)
		}
		if svccfg.Restart == "" {
			warnf("service %q: missing compose directive: restart; assuming 'unless-stopped' (add 'restart' to silence)", svccfg.Name)
		} else if svccfg.Restart != "always" && svccfg.Restart != "unless-stopped" {
			warnf("service %q: unsupported compose directive: restart; assuming 'unless-stopped' (add 'restart' to silence)", svccfg.Name)
		}
		if svccfg.ContainerName != "" {
			warnf("service %q: unsupported compose directive: container_name", svccfg.Name)
		}
		if svccfg.Hostname != "" {
			return fmt.Errorf("service %q: unsupported compose directive: hostname; consider using 'domainname' instead", svccfg.Name)
		}
		if len(svccfg.DNSSearch) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: dns_search", svccfg.Name)
		}
		if len(svccfg.DNSOpts) != 0 {
			warnf("service %q: unsupported compose directive: dns_opt", svccfg.Name)
		}
		if len(svccfg.DNS) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: dns", svccfg.Name)
		}
		if len(svccfg.Devices) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: devices", svccfg.Name)
		}
		if len(svccfg.DependsOn) != 0 {
			warnf("service %q: unsupported compose directive: depends_on", svccfg.Name)
		}
		if len(svccfg.DeviceCgroupRules) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: device_cgroup_rules", svccfg.Name)
		}
		if len(svccfg.Entrypoint) > 0 {
			return fmt.Errorf("service %q: unsupported compose directive: entrypoint", svccfg.Name)
		}
		if len(svccfg.GroupAdd) > 0 {
			return fmt.Errorf("service %q: unsupported compose directive: group_add", svccfg.Name)
		}
		if len(svccfg.Ipc) > 0 {
			warnf("service %q: unsupported compose directive: ipc", svccfg.Name)
		}
		if len(svccfg.Uts) > 0 {
			warnf("service %q: unsupported compose directive: uts", svccfg.Name)
		}
		if svccfg.Isolation != "" {
			warnf("service %q: unsupported compose directive: isolation", svccfg.Name)
		}
		if svccfg.MacAddress != "" {
			warnf("service %q: unsupported compose directive: mac_address", svccfg.Name)
		}
		if len(svccfg.Labels) > 0 {
			warnf("service %q: unsupported compose directive: labels", svccfg.Name) // TODO: add support for labels
		}
		if len(svccfg.Links) > 0 {
			warnf("service %q: unsupported compose directive: links", svccfg.Name)
		}
		if svccfg.Logging != nil {
			warnf("service %q: unsupported compose directive: logging", svccfg.Name)
		}
		for name := range svccfg.Networks {
			if _, ok := project.Networks[name]; !ok {
				warnf("service %q: network %q is not defined in the top-level networks section", svccfg.Name, name)
			}
		}
		for _, v := range svccfg.Volumes {
			if v.Type != "volume" {
				warnf("service %q: unsupported volume type: %q", svccfg.Name, v.Type)
				continue // TODO: ignore or error?
			}
			// Source=="" is allowed for anonymous volumes (will generate a random name)
			if _, ok := project.Volumes[v.Source]; !ok && v.Source != "" {
				warnf("service %q: volume %q is not defined in the top-level volumes section", svccfg.Name, v.Source)
			}
		}
		if len(svccfg.VolumesFrom) > 0 {
			warnf("service %q: unsupported compose directive: volumes_from", svccfg.Name) // TODO: add support for volumes_from
		}
		if svccfg.Build != nil {
			_, err := filepath.Abs(svccfg.Build.Context)
			if err != nil {
				return fmt.Errorf("service %q: invalid build context: %w", svccfg.Name, err)
			}
			if svccfg.Build.Dockerfile != "" {
				if filepath.IsAbs(svccfg.Build.Dockerfile) {
					return fmt.Errorf("service %q: dockerfile path must be relative to the build context: %q", svccfg.Name, svccfg.Build.Dockerfile)
				}
				if strings.HasPrefix(svccfg.Build.Dockerfile, "../") {
					return fmt.Errorf("service %q: dockerfile path must be inside the build context: %q", svccfg.Name, svccfg.Build.Dockerfile)
				}
				// Check if the dockerfile exists
				dockerfilePath := filepath.Join(svccfg.Build.Context, svccfg.Build.Dockerfile)
				if _, err := os.Stat(dockerfilePath); err != nil {
					return fmt.Errorf("service %q: dockerfile not found: %q", svccfg.Name, dockerfilePath)
				}
			}
			if svccfg.Build.SSH != nil {
				return fmt.Errorf("service %q: unsupported compose directive: build ssh", svccfg.Name)
			}
			if len(svccfg.Build.Labels) != 0 {
				warnf("service %q: unsupported compose directive: build labels", svccfg.Name) // TODO: add support for Kaniko --label
			}
			if len(svccfg.Build.CacheFrom) != 0 {
				warnf("service %q: unsupported compose directive: build cache_from", svccfg.Name)
			}
			if len(svccfg.Build.CacheTo) != 0 {
				warnf("service %q: unsupported compose directive: build cache_to", svccfg.Name)
			}
			if svccfg.Build.NoCache {
				warnf("service %q: unsupported compose directive: build no_cache", svccfg.Name)
			}
			if len(svccfg.Build.ExtraHosts) != 0 {
				return fmt.Errorf("service %q: unsupported compose directive: build extra_hosts", svccfg.Name)
			}
			if svccfg.Build.Isolation != "" {
				warnf("service %q: unsupported compose directive: build isolation", svccfg.Name)
			}
			if svccfg.Build.Network != "" {
				return fmt.Errorf("service %q: unsupported compose directive: build network", svccfg.Name)
			}
			if len(svccfg.Build.Secrets) != 0 {
				return fmt.Errorf("service %q: unsupported compose directive: build secrets", svccfg.Name) // TODO: support build secrets
			}
			if len(svccfg.Build.Tags) != 0 {
				return fmt.Errorf("service %q: unsupported compose directive: build tags", svccfg.Name)
			}
			if len(svccfg.Build.Platforms) != 0 {
				return fmt.Errorf("service %q: unsupported compose directive: build platforms", svccfg.Name)
			}
			if svccfg.Build.Privileged {
				return fmt.Errorf("service %q: unsupported compose directive: build privileged", svccfg.Name)
			}
			if svccfg.Build.DockerfileInline != "" {
				return fmt.Errorf("service %q: unsupported compose directive: build dockerfile_inline", svccfg.Name)
			}
		}
		for _, secret := range svccfg.Secrets {
			if !pkg.IsValidSecretName(secret.Source) {
				return fmt.Errorf("service %q: secret name is invalid: %q", svccfg.Name, secret.Source)
			}
			// secret.Target will always be automatically constructed by compose-go to "/run/secrets/<source>"
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
					warnf("service %q: ingress port without healthcheck defaults to GET / HTTP/1.1", svccfg.Name)
					break
				}
			}
		} else {
			timeout := 30 // default per compose spec
			if svccfg.HealthCheck.Timeout != nil {
				if *svccfg.HealthCheck.Timeout%1e9 != 0 {
					warnf("service %q: healthcheck timeout must be a multiple of 1s", svccfg.Name)
				}
				timeout = int(*svccfg.HealthCheck.Timeout / 1e9)
			}
			interval := 30 // default per compose spec
			if svccfg.HealthCheck.Interval != nil {
				if *svccfg.HealthCheck.Interval%1e9 != 0 {
					warnf("service %q: healthcheck interval must be a multiple of 1s", svccfg.Name)
				}
				interval = int(*svccfg.HealthCheck.Interval / 1e9)
			}
			// Technically this should test for <= but both interval and timeout have 30s as the default value
			if interval < timeout || timeout <= 0 {
				return fmt.Errorf("service %q: healthcheck timeout %ds must be positive and smaller than the interval %ds", svccfg.Name, timeout, interval)
			}
			if svccfg.HealthCheck.StartPeriod != nil {
				warnf("service %q: unsupported compose directive: healthcheck start_period", svccfg.Name)
			}
			if svccfg.HealthCheck.StartInterval != nil {
				warnf("service %q: unsupported compose directive: healthcheck start_interval", svccfg.Name)
			}
		}
		var reservations *compose.Resource
		if svccfg.Deploy != nil {
			if svccfg.Deploy.Mode != "" && svccfg.Deploy.Mode != "replicated" {
				return fmt.Errorf("service %q: unsupported compose directive: deploy mode: %q", svccfg.Name, svccfg.Deploy.Mode)
			}
			if svccfg.Deploy.UpdateConfig != nil {
				return fmt.Errorf("service %q: unsupported compose directive: deploy update_config", svccfg.Name)
			}
			if svccfg.Deploy.RollbackConfig != nil {
				return fmt.Errorf("service %q: unsupported compose directive: deploy rollback_config", svccfg.Name)
			}
			if svccfg.Deploy.RestartPolicy != nil {
				return fmt.Errorf("service %q: unsupported compose directive: deploy restart_policy", svccfg.Name)
			}
			if svccfg.Deploy.EndpointMode != "" {
				return fmt.Errorf("service %q: unsupported compose directive: deploy endpoint_mode", svccfg.Name)
			}
			if svccfg.Deploy.Resources.Limits != nil && svccfg.Deploy.Resources.Reservations == nil {
				warnf("service %q: no reservations specified; using limits as reservations", svccfg.Name)
			}
			reservations = getResourceReservations(svccfg.Deploy.Resources)
			if reservations != nil && reservations.NanoCPUs < 0 { // "0" just means "as small as possible"
				return fmt.Errorf("service %q: invalid value for cpus: %v", svccfg.Name, reservations.NanoCPUs)
			}
			if len(svccfg.Deploy.Labels) > 0 {
				warnf("service %q: unsupported compose directive: deploy labels", svccfg.Name)
			}
			if len(svccfg.Deploy.Placement.Constraints) != 0 || len(svccfg.Deploy.Placement.Preferences) != 0 || svccfg.Deploy.Placement.MaxReplicas != 0 {
				warnf("service %q: unsupported compose directive: deploy placement", svccfg.Name)
			}
		}
		if reservations == nil || reservations.MemoryBytes == 0 {
			warnf("service %q: missing memory reservation; specify deploy.resources.reservations.memory to avoid out-of-memory errors", svccfg.Name)
		}

		if dnsRoleVal := svccfg.Extensions["x-defang-dns-role"]; dnsRoleVal != nil {
			if _, ok := dnsRoleVal.(string); !ok {
				return fmt.Errorf("service %q: x-defang-dns-role must be a string", svccfg.Name)
			}
		}

		if staticFilesVal := svccfg.Extensions["x-defang-static-files"]; staticFilesVal != nil {
			_, str := staticFilesVal.(string)
			_, obj := staticFilesVal.(map[string]interface{})
			if !str && !obj {
				return fmt.Errorf("service %q: x-defang-static-files must be a string or object {folder: string, redirects: string[]}", svccfg.Name)
			}
		}

		if _, ok := svccfg.Extensions["x-defang-redis"]; ok {
			// Ensure the image is a valid Redis image
			repo := strings.SplitN(svccfg.Image, ":", 2)[0]
			if !strings.HasSuffix(repo, "redis") {
				warnf("service %q: managed Redis service should use a redis image", svccfg.Name)
			}
		}

		for k := range svccfg.Extensions {
			switch k {
			case "x-defang-dns-role", "x-defang-static-files", "x-defang-redis":
				continue
			default:
				warnf("service %q: unsupported compose extension: %q", svccfg.Name, k)
			}
		}
	}

	for k := range project.Extensions {
		warnf("unsupported compose extension: %q", k)
	}
	return nil
}

func warnf(format string, args ...interface{}) {
	logrus.Warnf(format, args...)
	term.SetHadWarnings(true)
}

func validatePorts(ports []compose.ServicePortConfig) error {
	for _, port := range ports {
		err := validatePort(port)
		if err != nil {
			return err
		}
	}
	return nil
}

// We can changed to slices.contains when we upgrade to go 1.21 or above
var validProtocols = map[string]bool{"": true, "tcp": true, "udp": true, "http": true, "http2": true, "grpc": true}
var validModes = map[string]bool{"": true, "host": true, "ingress": true}

func validatePort(port compose.ServicePortConfig) error {
	if port.Target < 1 || port.Target > 32767 {
		return fmt.Errorf("port 'target' must be an integer between 1 and 32767: %v", port.Target)
	}
	if port.HostIP != "" {
		return errors.New("port 'host_ip' is not supported")
	}
	if !validProtocols[port.Protocol] {
		return fmt.Errorf("port 'protocol' not one of [tcp udp http http2 grpc]: %v", port.Protocol)
	}
	if !validModes[port.Mode] {
		return fmt.Errorf("port 'mode' not one of [host ingress]: %v", port.Mode)
	}
	if port.Published != "" && (port.Mode == "host" || port.Protocol == "udp") {
		portRange := strings.SplitN(port.Published, "-", 2)
		start, err := strconv.ParseUint(portRange[0], 10, 16)
		if err != nil {
			return fmt.Errorf("port 'published' start must be an integer: %v", portRange[0])
		}
		if len(portRange) == 2 {
			end, err := strconv.ParseUint(portRange[1], 10, 16)
			if err != nil {
				return fmt.Errorf("port 'published' end must be an integer: %v", portRange[1])
			}
			if start > end {
				return fmt.Errorf("port 'published' start must be less than end: %v", port.Published)
			}
			if port.Target < uint32(start) || port.Target > uint32(end) {
				return fmt.Errorf("port 'published' range must include 'target': %v", port.Published)
			}
		} else {
			if start != uint64(port.Target) {
				return fmt.Errorf("port 'published' must be empty or equal to 'target': %v", port.Published)
			}
		}
	}

	return nil
}
