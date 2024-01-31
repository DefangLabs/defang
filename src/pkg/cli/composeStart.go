package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/sirupsen/logrus"
)

// ComposeStart reads a docker-compose.yml file and uploads the services to the fabric controller
func ComposeStart(ctx context.Context, client client.Client, filePath, projectName string, force bool) ([]*pb.ServiceInfo, error) {
	project, err := loadDockerCompose(filePath, projectName)
	if err != nil {
		return nil, &ComposeError{err}
	}

	for _, svccfg := range project.Services {
		normalized := NormalizeServiceName(svccfg.Name)
		if !pkg.IsValidServiceName(normalized) {
			// FIXME: this is too strict; we should allow more characters like underscores and dots
			return nil, &ComposeError{fmt.Errorf("service name is invalid: %q", svccfg.Name)}
		}
		if normalized != svccfg.Name {
			logrus.Warnf("service name %q was normalized to %q", svccfg.Name, normalized)
		}
		if svccfg.ContainerName != "" {
			logrus.Warn("unsupported compose directive: container_name")
		}
		if svccfg.Hostname != "" {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: hostname; consider using domainname instead")}
		}
		if len(svccfg.DNSSearch) != 0 {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: dns_search")}
		}
		if len(svccfg.DNSOpts) != 0 {
			logrus.Warn("unsupported compose directive: dns_opt")
		}
		if len(svccfg.DNS) != 0 {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: dns")}
		}
		if len(svccfg.Devices) != 0 {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: devices")}
		}
		if len(svccfg.DependsOn) != 0 {
			logrus.Warn("unsupported compose directive: depends_on")
		}
		if len(svccfg.DeviceCgroupRules) != 0 {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: device_cgroup_rules")}
		}
		if len(svccfg.Entrypoint) > 0 {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: entrypoint")}
		}
		if len(svccfg.GroupAdd) > 0 {
			return nil, &ComposeError{fmt.Errorf("unsupported compose directive: group_add")}
		}
		if len(svccfg.Ipc) > 0 {
			logrus.Warn("unsupported compose directive: ipc")
		}
		if len(svccfg.Uts) > 0 {
			logrus.Warn("unsupported compose directive: uts")
		}
		if svccfg.Isolation != "" {
			logrus.Warn("unsupported compose directive: isolation")
		}
		if svccfg.MacAddress != "" {
			logrus.Warn("unsupported compose directive: mac_address")
		}
		if len(svccfg.Labels) > 0 {
			logrus.Warn("unsupported compose directive: labels") // TODO: add support for labels
		}
		if len(svccfg.Links) > 0 {
			logrus.Warn("unsupported compose directive: links")
		}
		if svccfg.Logging != nil {
			logrus.Warn("unsupported compose directive: logging")
		}
		if _, ok := svccfg.Networks["default"]; !ok || len(svccfg.Networks) > 1 {
			logrus.Warn("unsupported compose directive: networks")
		}
		if len(svccfg.Volumes) > 0 {
			logrus.Warn("unsupported compose directive: volumes") // TODO: add support for volumes
		}
		if len(svccfg.VolumesFrom) > 0 {
			logrus.Warn("unsupported compose directive: volumes_from") // TODO: add support for volumes_from
		}
		if svccfg.Build != nil {
			if svccfg.Build.Dockerfile != "" {
				if filepath.IsAbs(svccfg.Build.Dockerfile) {
					return nil, &ComposeError{fmt.Errorf("dockerfile path must be relative to the build context: %q", svccfg.Build.Dockerfile)}
				}
				if strings.HasPrefix(svccfg.Build.Dockerfile, "../") {
					return nil, &ComposeError{fmt.Errorf("dockerfile path must be inside the build context: %q", svccfg.Build.Dockerfile)}
				}
				// Check if the dockerfile exists
				dockerfilePath := filepath.Join(svccfg.Build.Context, svccfg.Build.Dockerfile)
				if _, err := os.Stat(dockerfilePath); err != nil {
					return nil, &ComposeError{fmt.Errorf("dockerfile not found: %q", dockerfilePath)}
				}
			}
			if svccfg.Build.SSH != nil {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build ssh")}
			}
			if len(svccfg.Build.Labels) != 0 {
				logrus.Warn("unsupported compose directive: build labels") // TODO: add support for Kaniko --label
			}
			if len(svccfg.Build.CacheFrom) != 0 {
				logrus.Warn("unsupported compose directive: build cache_from")
			}
			if len(svccfg.Build.CacheTo) != 0 {
				logrus.Warn("unsupported compose directive: build cache_to")
			}
			if svccfg.Build.NoCache {
				logrus.Warn("unsupported compose directive: build no_cache")
			}
			if len(svccfg.Build.ExtraHosts) != 0 {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build extra_hosts")}
			}
			if svccfg.Build.Isolation != "" {
				logrus.Warn("unsupported compose directive: build isolation")
			}
			if svccfg.Build.Network != "" {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build network")}
			}
			if svccfg.Build.Target != "" {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build target")} // TODO: add support for Kaniko --target
			}
			if len(svccfg.Build.Secrets) != 0 {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build secrets")} // TODO: support build secrets
			}
			if len(svccfg.Build.Tags) != 0 {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build tags")}
			}
			if len(svccfg.Build.Platforms) != 0 {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build platforms")}
			}
			if svccfg.Build.Privileged {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build privileged")}
			}
			if svccfg.Build.DockerfileInline != "" {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: build dockerfile_inline")}
			}
		}
		for _, secret := range svccfg.Secrets {
			if !pkg.IsValidSecretName(secret.Source) {
				return nil, &ComposeError{fmt.Errorf("secret name is invalid: %q", secret.Source)}
			}
			if secret.Target != "" {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: secret target")}
			}
		}
		if svccfg.HealthCheck != nil && !svccfg.HealthCheck.Disable {
			timeout := 30 // default per compose spec
			if svccfg.HealthCheck.Timeout != nil {
				if *svccfg.HealthCheck.Timeout%1e9 != 0 {
					logrus.Warn("healthcheck timeout must be a multiple of 1s")
				}
				timeout = int(*svccfg.HealthCheck.Timeout / 1e9)
			}
			interval := 30 // default per compose spec
			if svccfg.HealthCheck.Interval != nil {
				if *svccfg.HealthCheck.Interval%1e9 != 0 {
					logrus.Warn("healthcheck interval must be a multiple of 1s")
				}
				interval = int(*svccfg.HealthCheck.Interval / 1e9)
			}
			// Technically this should test for <= but both interval and timeout have 30s as the default value
			if interval < timeout || timeout <= 0 {
				return nil, &ComposeError{fmt.Errorf("healthcheck timeout %ds must be positive and smaller than the interval %ds", timeout, interval)}
			}
			if svccfg.HealthCheck.StartPeriod != nil {
				logrus.Warn("unsupported compose directive: healthcheck start_period")
			}
			if svccfg.HealthCheck.StartInterval != nil {
				logrus.Warn("unsupported compose directive: healthcheck start_interval")
			}
		}
		if svccfg.Deploy != nil {
			if svccfg.Deploy.Mode != "" && svccfg.Deploy.Mode != "replicated" {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: deploy mode: %q", svccfg.Deploy.Mode)}
			}
			if len(svccfg.Deploy.Labels) > 0 {
				logrus.Warn("unsupported compose directive: deploy labels")
			}
			if svccfg.Deploy.UpdateConfig != nil {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: deploy update_config")}
			}
			if svccfg.Deploy.RollbackConfig != nil {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: deploy rollback_config")}
			}
			if svccfg.Deploy.RestartPolicy != nil {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: deploy restart_policy")}
			}
			if len(svccfg.Deploy.Placement.Constraints) != 0 || len(svccfg.Deploy.Placement.Preferences) != 0 || svccfg.Deploy.Placement.MaxReplicas != 0 {
				logrus.Warn("unsupported compose directive: deploy placement")
			}
			if svccfg.Deploy.EndpointMode != "" {
				return nil, &ComposeError{fmt.Errorf("unsupported compose directive: deploy endpoint_mode")}
			}
			if svccfg.Deploy.Resources.Limits != nil && svccfg.Deploy.Resources.Reservations == nil {
				logrus.Warn("no reservations specified; using limits as reservations")
			}
			reservations := getResourceReservations(svccfg.Deploy.Resources)
			if reservations != nil && reservations.NanoCPUs != "" {
				cpus, err := strconv.ParseFloat(reservations.NanoCPUs, 32)
				if err != nil || cpus < 0 { // "0" just means "as small as possible"
					return nil, &ComposeError{fmt.Errorf("invalid value for cpus: %q", reservations.NanoCPUs)}
				}
			}
		}
	}

	//
	// Publish updates
	//
	var services []*pb.Service
	for _, svccfg := range project.Services {
		var healthcheck *pb.HealthCheck
		if svccfg.HealthCheck != nil && len(svccfg.HealthCheck.Test) > 0 && !svccfg.HealthCheck.Disable {
			healthcheck = &pb.HealthCheck{
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

		ports, err := convertPorts(svccfg.Ports)
		if err != nil {
			// TODO: move this validation up so we don't upload the build context if it's invalid
			return nil, &ComposeError{err}
		}
		// Show a warning when we have ingress ports but no explicit healthcheck
		for _, port := range ports {
			if port.Mode == pb.Mode_INGRESS && healthcheck == nil {
				logrus.Warn("ingress port without healthcheck defaults to GET / HTTP/1.1")
				break
			}
		}

		var deploy *pb.Deploy
		if svccfg.Deploy != nil {
			deploy = &pb.Deploy{}
			deploy.Replicas = 1 // default to one replica per service; allow the user to override this to 0
			if svccfg.Deploy.Replicas != nil {
				deploy.Replicas = uint32(*svccfg.Deploy.Replicas)
			}

			reservations := getResourceReservations(svccfg.Deploy.Resources)
			if reservations != nil {
				cpus := 0.0
				if reservations.NanoCPUs != "" {
					cpus, err = strconv.ParseFloat(reservations.NanoCPUs, 32)
					if err != nil {
						panic(err) // was already validated above
					}
				}
				var devices []*pb.Device
				for _, d := range reservations.Devices {
					devices = append(devices, &pb.Device{
						Capabilities: d.Capabilities,
						Count:        uint32(d.Count),
						Driver:       d.Driver,
					})
				}
				deploy.Resources = &pb.Resources{
					Reservations: &pb.Resource{
						Cpus:    float32(cpus),
						Memory:  float32(reservations.MemoryBytes) / MiB,
						Devices: devices,
					},
				}
			}
		}

		if deploy == nil || deploy.Resources == nil || deploy.Resources.Reservations == nil || deploy.Resources.Reservations.Memory == 0 {
			logrus.Warn("missing memory reservation; specify deploy.resources.reservations.memory to avoid out-of-memory errors")
		}

		// Upload the build context, if any; TODO: parallelize
		var build *pb.Build
		if svccfg.Build != nil {
			// Pack the build context into a tarball and upload
			url, err := getRemoteBuildContext(ctx, client, svccfg.Name, svccfg.Build, force)
			if err != nil {
				return nil, err
			}

			build = &pb.Build{
				Context:    url,
				Dockerfile: svccfg.Build.Dockerfile,
				ShmSize:    float32(svccfg.Build.ShmSize) / MiB,
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
		var secrets []*pb.Secret
		for _, secret := range svccfg.Secrets {
			secrets = append(secrets, &pb.Secret{
				Source: secret.Source,
			})
		}

		init := false
		if svccfg.Init != nil {
			init = *svccfg.Init
		}

		services = append(services, &pb.Service{
			Name:        NormalizeServiceName(svccfg.Name),
			Image:       svccfg.Image,
			Build:       build,
			Internal:    true, // TODO: support external services (w/o LB)
			Init:        init,
			Ports:       ports,
			Healthcheck: healthcheck,
			Deploy:      deploy,
			Environment: envs,
			Secrets:     secrets,
			Command:     svccfg.Command,
			Domainname:  svccfg.DomainName,
			Platform:    convertPlatform(svccfg.Platform),
		})
	}

	if len(services) == 0 {
		return nil, &ComposeError{fmt.Errorf("no services found")}
	}

	if DoDryRun {
		for _, service := range services {
			PrintObject(service.Name, service)
		}
		return nil, nil
	}

	for _, service := range services {
		Info(" * Deploying service", service.Name)
	}

	resp, err := client.Deploy(ctx, &pb.DeployRequest{
		Services: services,
	})
	if err != nil {
		return nil, err
	}

	if DoVerbose {
		for _, service := range resp.Services {
			PrintObject(service.Service.Name, service)
		}
	}
	return resp.Services, nil
}

func getResourceReservations(r types.Resources) *types.Resource {
	if r.Reservations == nil {
		// TODO: we might not want to default to all the limits, maybe only memory?
		return r.Limits
	}
	return r.Reservations
}
