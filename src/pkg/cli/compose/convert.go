package compose

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func ConvertServices(ctx context.Context, c client.Client, serviceConfigs compose.Services, force BuildContext) ([]*defangv1.Service, error) {
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

	// Preload the current config so we can detect which environment variables should be passed as "secrets"
	config, err := c.ListConfig(ctx)
	if err != nil {
		term.Debugf("failed to load config: %v", err)
		config = &defangv1.Secrets{}
	}
	slices.Sort(config.Names) // sort for binary search

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
						Cpus:    float32(reservations.NanoCPUs),
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
				build.Args = make(map[string]string, len(svccfg.Build.Args))
				for key, value := range svccfg.Build.Args {
					if key == "" || value == nil {
						warnf("service %q: skipping unset build argument %q", svccfg.Name, key)
						continue
					}
					build.Args[key] = *value
				}
			}
		}

		// Extract environment variables
		var envFromConfig []string
		envs := make(map[string]string, len(svccfg.Environment))
		for key, value := range svccfg.Environment {
			// A bug in Compose-go env file parsing can cause empty keys
			if key == "" {
				warnf("service %q: skipping unset environment variable key", svccfg.Name)
				continue
			}
			// keep track of what environment variables were declared but not set in the compose environment section
			if value == nil {
				envFromConfig = append(envFromConfig, key)
				continue
			}

			// Check if the environment variable is an existing config; if so, mark it as such
			if _, ok := slices.BinarySearch(config.Names, key); ok {
				term.Warnf("service %q: environment variable %q overridden by config", svccfg.Name, key)
				envFromConfig = append(envFromConfig, key)
				continue
			}

			val := *value
			if serviceNameRegex != nil {
				// Replace service names with their actual DNS names; TODO: support public names too
				val = serviceNameRegex.ReplaceAllStringFunc(*value, func(serviceName string) string {
					return c.ServiceDNS(NormalizeServiceName(serviceName))
				})
				if val != *value {
					warnf("service %q: service names were replaced in environment variable %q: %q", svccfg.Name, key, val)
				}
			}
			envs[key] = val
		}

		// Add unset environment variables as "secrets"
		var configs []*defangv1.Secret
		for _, name := range envFromConfig {
			configs = append(configs, &defangv1.Secret{
				Source: name,
			})
		}
		// Extract secret references; secrets are supposed to be files, not env, but it's kept for backward compatibility
		for i, secret := range svccfg.Secrets {
			if i == 0 { // only warn once
				warnf("service %q: secrets will be exposed as environment variables, not files (use 'environment' instead)", svccfg.Name)
			}
			configs = append(configs, &defangv1.Secret{
				Source: secret.Source,
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
		if _, ok := svccfg.Extensions["x-defang-redis"]; ok {
			redis = &defangv1.Redis{}
		}

		if redis == nil && isStatefulImage(svccfg.Image) {
			warnf("service %q: stateful service will lose data on restart; use a managed service instead", svccfg.Name)
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

func getResourceReservations(r compose.Resources) *compose.Resource {
	if r.Reservations == nil {
		// TODO: we might not want to default to all the limits, maybe only memory?
		return r.Limits
	}
	return r.Reservations
}

func convertPlatform(platform string) defangv1.Platform {
	switch strings.ToLower(platform) {
	default:
		warnf("unsupported platform: %q; assuming linux", platform)
		fallthrough
	case "", "linux":
		return defangv1.Platform_LINUX_ANY
	case "linux/amd64", "linux/x86_64": // Docker accepts both
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

func convertPort(port compose.ServicePortConfig) *defangv1.Port {
	pbPort := &defangv1.Port{
		// Mode      string `yaml:",omitempty" json:"mode,omitempty"`
		// HostIP    string `mapstructure:"host_ip" yaml:"host_ip,omitempty" json:"host_ip,omitempty"`
		// Published string `yaml:",omitempty" json:"published,omitempty"`
		// Protocol  string `yaml:",omitempty" json:"protocol,omitempty"`
		Target: port.Target,
	}

	switch port.Protocol {
	case "":
		pbPort.Protocol = defangv1.Protocol_ANY // defaults to HTTP in CD
	case "tcp":
		pbPort.Protocol = defangv1.Protocol_TCP
	case "udp":
		pbPort.Protocol = defangv1.Protocol_UDP
	case "http": // TODO: not per spec
		pbPort.Protocol = defangv1.Protocol_HTTP
	case "http2": // TODO: not per spec
		pbPort.Protocol = defangv1.Protocol_HTTP2
	case "grpc": // TODO: not per spec
		pbPort.Protocol = defangv1.Protocol_GRPC
	default:
		panic(fmt.Sprintf("port 'protocol' should have been validated to be one of [tcp udp http http2 grpc] but got: %v", port.Protocol))
	}

	switch port.Mode {
	case "":
		warnf("No port 'mode' was specified; defaulting to 'ingress' (add 'mode: ingress' to silence)")
		fallthrough
	case "ingress":
		// This code is unnecessarily complex because compose-go silently converts short port: syntax to ingress+tcp
		if port.Protocol != "udp" {
			if port.Published != "" {
				warnf("Published ports are ignored in ingress mode")
			}
			pbPort.Mode = defangv1.Mode_INGRESS
			if pbPort.Protocol == defangv1.Protocol_TCP || pbPort.Protocol == defangv1.Protocol_UDP {
				warnf("TCP ingress is not supported; assuming HTTP (remove 'protocol' to silence)")
				pbPort.Protocol = defangv1.Protocol_HTTP
			}
			break
		}
		warnf("UDP ports default to 'host' mode (add 'mode: host' to silence)")
		fallthrough
	case "host":
		pbPort.Mode = defangv1.Mode_HOST
	default:
		panic(fmt.Sprintf("port mode should have been validated to be one of [host ingress] but got: %v", port.Mode))
	}
	return pbPort
}

func convertPorts(ports []compose.ServicePortConfig) []*defangv1.Port {
	var pbports []*defangv1.Port
	for _, port := range ports {
		pbPort := convertPort(port)
		pbports = append(pbports, pbPort)
	}
	return pbports
}

func convertServiceState(state string) defangv1.ServiceState {
	switch strings.ToLower(state) {
	default:
		return defangv1.ServiceState_UNKNOWN
	case "building_pending":
		return defangv1.ServiceState_BUILD_PENDING
	case "building":
		return defangv1.ServiceState_BUILDING
	case "created":
		return defangv1.ServiceState_CREATED
	case "update_pending":
		return defangv1.ServiceState_UPDATE_PENDING
	case "service_deploying":
		return defangv1.ServiceState_DEPLOYING
	case "service_started":
		return defangv1.ServiceState_STARTED
	case "service_failed":
		return defangv1.ServiceState_FAILED
	}
}
