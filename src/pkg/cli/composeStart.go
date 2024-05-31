package cli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func convertServices(ctx context.Context, c client.Client, serviceConfigs compose.Services, force bool) ([]*defangv1.Service, error) {
	// Create a regexp to detect private service names in environment variable values
	var serviceNames []string
	for _, svccfg := range serviceConfigs {
		if network(&svccfg) == defangv1.Network_PRIVATE && slices.ContainsFunc(svccfg.Ports, func(p compose.ServicePortConfig) bool {
			return p.Mode == "host" // only private services with host ports get DNS names
		}) {
			serviceNames = append(serviceNames, regexp.QuoteMeta(svccfg.Name))
		}
	}
	var serviceNameRegex *regexp.Regexp
	if len(serviceNames) > 0 {
		serviceNameRegex = regexp.MustCompile(`\b(?:` + strings.Join(serviceNames, "|") + `)\b`)
	}

	//
	// Publish updates
	//
	var services []*defangv1.Service
	for _, svccfg := range serviceConfigs {
		var healthcheck *defangv1.HealthCheck
		if svccfg.HealthCheck != nil && len(svccfg.HealthCheck.Test) > 0 && !svccfg.HealthCheck.Disable {
			healthcheck = &defangv1.HealthCheck{
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

		var deploy *defangv1.Deploy
		if svccfg.Deploy != nil {
			deploy = &defangv1.Deploy{}
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
						panic(err) // was already validated
					}
				}
				var devices []*defangv1.Device
				for _, d := range reservations.Devices {
					devices = append(devices, &defangv1.Device{
						Capabilities: d.Capabilities,
						Count:        uint32(d.Count),
						Driver:       d.Driver,
					})
				}
				deploy.Resources = &defangv1.Resources{
					Reservations: &defangv1.Resource{
						Cpus:    float32(cpus),
						Memory:  float32(reservations.MemoryBytes) / MiB,
						Devices: devices,
					},
				}
			}
		}

		// Upload the build context, if any; TODO: parallelize
		var build *defangv1.Build
		if svccfg.Build != nil {
			// Pack the build context into a tarball and upload
			url, err := getRemoteBuildContext(ctx, c, svccfg.Name, svccfg.Build, force)
			if err != nil {
				return nil, err
			}

			build = &defangv1.Build{
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
		unsetEnvs := []string{}
		envs := make(map[string]string)
		for key, value := range svccfg.Environment {
			if value == nil {
				value = resolveEnv(key)
			}

			// keep track of what environment variables were declared but not set in the compose environment section
			if value == nil {
				unsetEnvs = append(unsetEnvs, key)
				continue
			}

			val := *value
			if serviceNameRegex != nil {
				// Replace service names with their actual DNS names
				val = serviceNameRegex.ReplaceAllStringFunc(*value, func(serviceName string) string {
					return c.ServiceDNS(NormalizeServiceName(serviceName))
				})
				if val != *value {
					warnf("service names were replaced in environment variable %q: %q", key, val)
				}
			}
			envs[key] = val
		}

		// Extract secret references
		var configs []*defangv1.Secret
		for i, secret := range svccfg.Secrets {
			if i == 0 {
				warnf("secrets will be exposed as environment variables, not files (use 'environment' to silence)")
			}
			configs = append(configs, &defangv1.Secret{
				Source: secret.Source,
			})
		}
		// add unset environment variables as secrets
		for _, unsetEnv := range unsetEnvs {
			configs = append(configs, &defangv1.Secret{
				Source: unsetEnv,
			})
		}

		init := false
		if svccfg.Init != nil {
			init = *svccfg.Init
		}

		var dnsRole string
		if dnsRoleVal := svccfg.Extensions["x-defang-dns-role"]; dnsRoleVal != nil {
			dnsRole = dnsRoleVal.(string) // already validated
		}

		var staticFiles *defangv1.StaticFiles
		if staticFilesVal := svccfg.Extensions["x-defang-static-files"]; staticFilesVal != nil {
			if str, ok := staticFilesVal.(string); ok {
				staticFiles = &defangv1.StaticFiles{Folder: str}
			} else {
				obj := staticFilesVal.(map[string]interface{}) // already validated
				var redirects []string
				if r, ok := obj["redirects"].([]interface{}); ok {
					redirects = make([]string, len(r))
					for i, v := range r {
						redirects[i] = v.(string)
					}
				}
				staticFiles = &defangv1.StaticFiles{
					Folder:    obj["folder"].(string),
					Redirects: redirects,
				}
			}
		}

		var redis *defangv1.Redis
		if redisCacheVal := svccfg.Extensions["x-defang-redis"]; redisCacheVal != nil {
			redis = &defangv1.Redis{}
		}

		network := network(&svccfg)
		ports := convertPorts(svccfg.Ports)
		services = append(services, &defangv1.Service{
			Name:        NormalizeServiceName(svccfg.Name),
			Image:       svccfg.Image,
			Build:       build,
			Internal:    network == defangv1.Network_PRIVATE,
			Networks:    network,
			Init:        init,
			Ports:       ports,
			Healthcheck: healthcheck,
			Deploy:      deploy,
			Environment: envs,
			Secrets:     configs,
			Command:     svccfg.Command,
			Domainname:  svccfg.DomainName,
			Platform:    convertPlatform(svccfg.Platform),
			DnsRole:     dnsRole,
			StaticFiles: staticFiles,
			Redis:       redis,
		})
	}
	return services, nil
}

// ComposeStart validates a compose project and uploads the services using the client
func ComposeStart(ctx context.Context, c client.Client, force bool) (*defangv1.DeployResponse, error) {
	project, err := c.LoadProject()
	if err != nil {
		return nil, err
	}

	if err := validateProject(project); err != nil {
		return nil, &ComposeError{err}
	}

	services, err := convertServices(ctx, c, project.Services, force)
	if err != nil {
		return nil, err
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
		term.Info(" * Deploying service", service.Name)
	}

	resp, err := c.Deploy(ctx, &defangv1.DeployRequest{
		Services: services,
	})
	if err != nil {
		return nil, err
	}

	if term.DoDebug {
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

func resolveEnv(k string) *string {
	// TODO: per spec, if the value is nil, then the value is taken from an interactive prompt
	v, ok := os.LookupEnv(k)
	if !ok {
		warnf("environment variable not found: %q; using config", k)
		// If the value could not be resolved, it should be removed
		return nil
	}
	return &v
}

func convertPlatform(platform string) defangv1.Platform {
	switch platform {
	default:
		warnf("Unsupported platform: %q (assuming linux)", platform)
		fallthrough
	case "", "linux":
		return defangv1.Platform_LINUX_ANY
	case "linux/amd64":
		return defangv1.Platform_LINUX_AMD64
	case "linux/arm64", "linux/arm64/v8", "linux/arm64/v7", "linux/arm64/v6":
		return defangv1.Platform_LINUX_ARM64
	}
}

func network(svccfg *compose.ServiceConfig) defangv1.Network {
	// HACK: Use magic network name "public" to determine if the service is public
	if _, ok := svccfg.Networks["public"]; ok {
		return defangv1.Network_PUBLIC
	}
	// TODO: support external services (w/o LB),
	return defangv1.Network_PRIVATE
}
