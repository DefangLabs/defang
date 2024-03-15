package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/sirupsen/logrus"
)

func validateProject(project *compose.Project) error {
	if project == nil {
		return &ComposeError{errors.New("no project found")}
	}
	for _, svccfg := range project.Services {
		normalized := NormalizeServiceName(svccfg.Name)
		if !pkg.IsValidServiceName(normalized) {
			// FIXME: this is too strict; we should allow more characters like underscores and dots
			return &ComposeError{fmt.Errorf("service name is invalid: %q", svccfg.Name)}
		}
		if normalized != svccfg.Name {
			logrus.Warnf("service name %q was normalized to %q", svccfg.Name, normalized)
			HadWarnings = true
		}
		if svccfg.ContainerName != "" {
			logrus.Warn("unsupported compose directive: container_name")
			HadWarnings = true
		}
		if svccfg.Hostname != "" {
			return &ComposeError{fmt.Errorf("unsupported compose directive: hostname; consider using domainname instead")}
		}
		if len(svccfg.DNSSearch) != 0 {
			return &ComposeError{fmt.Errorf("unsupported compose directive: dns_search")}
		}
		if len(svccfg.DNSOpts) != 0 {
			logrus.Warn("unsupported compose directive: dns_opt")
			HadWarnings = true
		}
		if len(svccfg.DNS) != 0 {
			return &ComposeError{fmt.Errorf("unsupported compose directive: dns")}
		}
		if len(svccfg.Devices) != 0 {
			return &ComposeError{fmt.Errorf("unsupported compose directive: devices")}
		}
		if len(svccfg.DependsOn) != 0 {
			logrus.Warn("unsupported compose directive: depends_on")
			HadWarnings = true
		}
		if len(svccfg.DeviceCgroupRules) != 0 {
			return &ComposeError{fmt.Errorf("unsupported compose directive: device_cgroup_rules")}
		}
		if len(svccfg.Entrypoint) > 0 {
			return &ComposeError{fmt.Errorf("unsupported compose directive: entrypoint")}
		}
		if len(svccfg.GroupAdd) > 0 {
			return &ComposeError{fmt.Errorf("unsupported compose directive: group_add")}
		}
		if len(svccfg.Ipc) > 0 {
			logrus.Warn("unsupported compose directive: ipc")
			HadWarnings = true
		}
		if len(svccfg.Uts) > 0 {
			logrus.Warn("unsupported compose directive: uts")
			HadWarnings = true
		}
		if svccfg.Isolation != "" {
			logrus.Warn("unsupported compose directive: isolation")
			HadWarnings = true
		}
		if svccfg.MacAddress != "" {
			logrus.Warn("unsupported compose directive: mac_address")
			HadWarnings = true
		}
		if len(svccfg.Labels) > 0 {
			logrus.Warn("unsupported compose directive: labels") // TODO: add support for labels
			HadWarnings = true
		}
		if len(svccfg.Links) > 0 {
			logrus.Warn("unsupported compose directive: links")
			HadWarnings = true
		}
		if svccfg.Logging != nil {
			logrus.Warn("unsupported compose directive: logging")
			HadWarnings = true
		}
		for name, _ := range svccfg.Networks {
			if _, ok := project.Networks[name]; !ok {
				logrus.Warnf("network %v used by service %v is not defined in the top-level networks section", name, svccfg.Name)
				HadWarnings = true
			}
		}
		if len(svccfg.Volumes) > 0 {
			logrus.Warn("unsupported compose directive: volumes") // TODO: add support for volumes
			HadWarnings = true
		}
		if len(svccfg.VolumesFrom) > 0 {
			logrus.Warn("unsupported compose directive: volumes_from") // TODO: add support for volumes_from
			HadWarnings = true
		}
		if svccfg.Build != nil {
			if svccfg.Build.Dockerfile != "" {
				if filepath.IsAbs(svccfg.Build.Dockerfile) {
					return &ComposeError{fmt.Errorf("dockerfile path must be relative to the build context: %q", svccfg.Build.Dockerfile)}
				}
				if strings.HasPrefix(svccfg.Build.Dockerfile, "../") {
					return &ComposeError{fmt.Errorf("dockerfile path must be inside the build context: %q", svccfg.Build.Dockerfile)}
				}
				// Check if the dockerfile exists
				dockerfilePath := filepath.Join(svccfg.Build.Context, svccfg.Build.Dockerfile)
				if _, err := os.Stat(dockerfilePath); err != nil {
					return &ComposeError{fmt.Errorf("dockerfile not found: %q", dockerfilePath)}
				}
			}
			if svccfg.Build.SSH != nil {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build ssh")}
			}
			if len(svccfg.Build.Labels) != 0 {
				logrus.Warn("unsupported compose directive: build labels") // TODO: add support for Kaniko --label
				HadWarnings = true
			}
			if len(svccfg.Build.CacheFrom) != 0 {
				logrus.Warn("unsupported compose directive: build cache_from")
				HadWarnings = true
			}
			if len(svccfg.Build.CacheTo) != 0 {
				logrus.Warn("unsupported compose directive: build cache_to")
				HadWarnings = true
			}
			if svccfg.Build.NoCache {
				logrus.Warn("unsupported compose directive: build no_cache")
				HadWarnings = true
			}
			if len(svccfg.Build.ExtraHosts) != 0 {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build extra_hosts")}
			}
			if svccfg.Build.Isolation != "" {
				logrus.Warn("unsupported compose directive: build isolation")
				HadWarnings = true
			}
			if svccfg.Build.Network != "" {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build network")}
			}
			if len(svccfg.Build.Secrets) != 0 {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build secrets")} // TODO: support build secrets
			}
			if len(svccfg.Build.Tags) != 0 {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build tags")}
			}
			if len(svccfg.Build.Platforms) != 0 {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build platforms")}
			}
			if svccfg.Build.Privileged {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build privileged")}
			}
			if svccfg.Build.DockerfileInline != "" {
				return &ComposeError{fmt.Errorf("unsupported compose directive: build dockerfile_inline")}
			}
		}
		for _, secret := range svccfg.Secrets {
			if !pkg.IsValidSecretName(secret.Source) {
				return &ComposeError{fmt.Errorf("secret name is invalid: %q", secret.Source)}
			}
			if secret.Target != "" {
				return &ComposeError{fmt.Errorf("unsupported compose directive: secret target")}
			}
			if s, ok := project.Secrets[secret.Source]; !ok {
				logrus.Warnf("secret %q is not defined in the top-level secrets section", secret.Source)
			} else if s.Name != "" && s.Name != secret.Source {
				return &ComposeError{fmt.Errorf("unsupported secret %q: cannot override name %q", secret.Source, s.Name)} // TODO: support custom secret names
			} else if !s.External {
				logrus.Warnf("unsupported secret %q: not marked external:true", secret.Source) // TODO: support secrets from environment/file
			}
		}
		err := validatePorts(svccfg.Ports)
		if err != nil {
			return &ComposeError{err}
		}
		if svccfg.HealthCheck == nil || svccfg.HealthCheck.Disable {
			// Show a warning when we have ingress ports but no explicit healthcheck
			for _, port := range svccfg.Ports {
				if port.Mode == "ingress" {
					logrus.Warn("ingress port without healthcheck defaults to GET / HTTP/1.1")
					HadWarnings = true
					break
				}
			}
		} else {
			timeout := 30 // default per compose spec
			if svccfg.HealthCheck.Timeout != nil {
				if *svccfg.HealthCheck.Timeout%1e9 != 0 {
					logrus.Warn("healthcheck timeout must be a multiple of 1s")
					HadWarnings = true
				}
				timeout = int(*svccfg.HealthCheck.Timeout / 1e9)
			}
			interval := 30 // default per compose spec
			if svccfg.HealthCheck.Interval != nil {
				if *svccfg.HealthCheck.Interval%1e9 != 0 {
					logrus.Warn("healthcheck interval must be a multiple of 1s")
					HadWarnings = true
				}
				interval = int(*svccfg.HealthCheck.Interval / 1e9)
			}
			// Technically this should test for <= but both interval and timeout have 30s as the default value
			if interval < timeout || timeout <= 0 {
				return &ComposeError{fmt.Errorf("healthcheck timeout %ds must be positive and smaller than the interval %ds", timeout, interval)}
			}
			if svccfg.HealthCheck.StartPeriod != nil {
				logrus.Warn("unsupported compose directive: healthcheck start_period")
				HadWarnings = true
			}
			if svccfg.HealthCheck.StartInterval != nil {
				logrus.Warn("unsupported compose directive: healthcheck start_interval")
				HadWarnings = true
			}
		}
		if svccfg.Deploy != nil {
			if svccfg.Deploy.Mode != "" && svccfg.Deploy.Mode != "replicated" {
				return &ComposeError{fmt.Errorf("unsupported compose directive: deploy mode: %q", svccfg.Deploy.Mode)}
			}
			if len(svccfg.Deploy.Labels) > 0 {
				logrus.Warn("unsupported compose directive: deploy labels")
				HadWarnings = true
			}
			if svccfg.Deploy.UpdateConfig != nil {
				return &ComposeError{fmt.Errorf("unsupported compose directive: deploy update_config")}
			}
			if svccfg.Deploy.RollbackConfig != nil {
				return &ComposeError{fmt.Errorf("unsupported compose directive: deploy rollback_config")}
			}
			if svccfg.Deploy.RestartPolicy != nil {
				return &ComposeError{fmt.Errorf("unsupported compose directive: deploy restart_policy")}
			}
			if len(svccfg.Deploy.Placement.Constraints) != 0 || len(svccfg.Deploy.Placement.Preferences) != 0 || svccfg.Deploy.Placement.MaxReplicas != 0 {
				logrus.Warn("unsupported compose directive: deploy placement")
				HadWarnings = true
			}
			if svccfg.Deploy.EndpointMode != "" {
				return &ComposeError{fmt.Errorf("unsupported compose directive: deploy endpoint_mode")}
			}
			if svccfg.Deploy.Resources.Limits != nil && svccfg.Deploy.Resources.Reservations == nil {
				logrus.Warn("no reservations specified; using limits as reservations")
				HadWarnings = true
			}
			reservations := getResourceReservations(svccfg.Deploy.Resources)
			if reservations != nil && reservations.NanoCPUs != "" {
				cpus, err := strconv.ParseFloat(reservations.NanoCPUs, 32)
				if err != nil || cpus < 0 { // "0" just means "as small as possible"
					return &ComposeError{fmt.Errorf("invalid value for cpus: %q", reservations.NanoCPUs)}
				}
			}
		}
		var reservations *compose.Resource
		if svccfg.Deploy != nil {
			reservations = getResourceReservations(svccfg.Deploy.Resources)
		}

		if svccfg.Deploy == nil || reservations == nil || reservations.MemoryBytes == 0 {
			logrus.Warn("missing memory reservation; specify deploy.resources.reservations.memory to avoid out-of-memory errors")
			HadWarnings = true
		}

	}
	return nil
}

// ComposeStart validates a compose project and uploads the services using the client
func ComposeStart(ctx context.Context, c client.Client, project *compose.Project, force bool) (*v1.DeployResponse, error) {
	if err := validateProject(project); err != nil {
		return nil, err
	}
	//
	// Publish updates
	//
	var services []*v1.Service
	for _, svccfg := range project.Services {
		var healthcheck *v1.HealthCheck
		if svccfg.HealthCheck != nil && len(svccfg.HealthCheck.Test) > 0 && !svccfg.HealthCheck.Disable {
			healthcheck = &v1.HealthCheck{
				Test: svccfg.HealthCheck.Test,
			}
			if nil != svccfg.HealthCheck.Interval {
				healthcheck.Interval = uint32(*svccfg.HealthCheck.Interval / 1e9)
			}
			if nil != svccfg.HealthCheck.Timeout {
				healthcheck.Timeout = uint32(*svccfg.HealthCheck.Timeout / 1e9)
			}
			if nil != svccfg.HealthCheck.Retries {
				healthcheck.Retries = uint32(*svccfg.HealthCheck.Retries)
			}
		}

		var deploy *v1.Deploy
		if svccfg.Deploy != nil {
			deploy = &v1.Deploy{}
			deploy.Replicas = 1 // default to one replica per service; allow the user to override this to 0
			if svccfg.Deploy.Replicas != nil {
				deploy.Replicas = uint32(*svccfg.Deploy.Replicas)
			}

			reservations := getResourceReservations(svccfg.Deploy.Resources)
			if reservations != nil {
				cpus := 0.0
				var err error
				if reservations.NanoCPUs != "" {
					cpus, err = strconv.ParseFloat(reservations.NanoCPUs, 32)
					if err != nil {
						panic(err) // was already validated above
					}
				}
				var devices []*v1.Device
				for _, d := range reservations.Devices {
					devices = append(devices, &v1.Device{
						Capabilities: d.Capabilities,
						Count:        uint32(d.Count),
						Driver:       d.Driver,
					})
				}
				deploy.Resources = &v1.Resources{
					Reservations: &v1.Resource{
						Cpus:    float32(cpus),
						Memory:  float32(reservations.MemoryBytes) / MiB,
						Devices: devices,
					},
				}
			}
		}

		// Upload the build context, if any; TODO: parallelize
		var build *v1.Build
		if svccfg.Build != nil {
			// Pack the build context into a tarball and upload
			url, err := getRemoteBuildContext(ctx, c, svccfg.Name, svccfg.Build, force)
			if err != nil {
				return nil, err
			}

			build = &v1.Build{
				Context:    url,
				Dockerfile: svccfg.Build.Dockerfile,
				ShmSize:    float32(svccfg.Build.ShmSize) / MiB,
				Target:     svccfg.Build.Target,
			}

			if len(svccfg.Build.Args) > 0 {
				build.Args = make(map[string]string)
				for key, value := range svccfg.Build.Args {
					if value == nil {
						value = resolveEnv(key)
					}
					if value != nil {
						build.Args[key] = *value
					}
				}
			}
		}

		// Extract environment variables
		envs := make(map[string]string)
		for key, value := range svccfg.Environment {
			if value == nil {
				value = resolveEnv(key)
			}
			if value != nil {
				envs[key] = *value
			}
		}

		// Extract secret references
		var secrets []*v1.Secret
		for _, secret := range svccfg.Secrets {
			secrets = append(secrets, &v1.Secret{
				Source: secret.Source,
			})
		}

		init := false
		if svccfg.Init != nil {
			init = *svccfg.Init
		}

		var dnsRole string
		dnsRoleVal := svccfg.Extensions["x-dns-role"]
		if dnsRoleVal != nil {
			dnsRole = fmt.Sprint(dnsRoleVal)
		}

		// Hack: Use magic network name "public" to determine if the service is private
		privateNetwork := true
		if _, ok := svccfg.Networks["public"]; ok {
			privateNetwork = false
		}

		ports, err := convertPorts(svccfg.Ports)
		if err != nil {
			// This should never happen as we have validated above
			return nil, &ComposeError{err}
		}
		services = append(services, &v1.Service{
			Name:        NormalizeServiceName(svccfg.Name),
			Image:       svccfg.Image,
			Build:       build,
			Internal:    privateNetwork, // TODO: support external services (w/o LB)
			Init:        init,
			Ports:       ports,
			Healthcheck: healthcheck,
			Deploy:      deploy,
			Environment: envs,
			Secrets:     secrets,
			Command:     svccfg.Command,
			Domainname:  svccfg.DomainName,
			Platform:    convertPlatform(svccfg.Platform),
			DnsRole:     dnsRole,
		})
	}

	if len(services) == 0 {
		return nil, &ComposeError{fmt.Errorf("no services found")}
	}

	if DoDryRun {
		for _, service := range services {
			PrintObject(service.Name, service)
		}
		return nil, ErrDryRun
	}

	for _, service := range services {
		Info(" * Deploying service", service.Name)
	}

	resp, err := c.Deploy(ctx, &v1.DeployRequest{
		Services: services,
	})
	var warnings client.Warnings
	if errors.As(err, &warnings) {
		if len(warnings) > 0 {
			Warn(" !", warnings)
		}
	} else if err != nil {
		return nil, err
	}

	if DoDebug {
		for _, service := range resp.Services {
			PrintObject(service.Service.Name, service)
		}
	}
	return resp, nil
}

func getResourceReservations(r compose.Resources) *compose.Resource {
	if r.Reservations == nil {
		// TODO: we might not want to default to all the limits, maybe only memory?
		return r.Limits
	}
	return r.Reservations
}
