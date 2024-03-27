package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

// ComposeStart validates a compose project and uploads the services using the client
func ComposeStart(ctx context.Context, c client.Client, project *compose.Project, force bool) (*v1.DeployResponse, error) {
	if err := validateProject(project); err != nil {
		return nil, &ComposeError{err}
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
		if dnsRoleVal := svccfg.Extensions["x-defang-dns-role"]; dnsRoleVal != nil {
			dnsRole = dnsRoleVal.(string) // already validated above
		}

		var staticFiles string
		if staticFilesVal := svccfg.Extensions["x-defang-static-files"]; staticFilesVal != nil {
			staticFiles = staticFilesVal.(string) // already validated above
		}

		// Hack: Use magic network name "public" to determine if the service is private
		privateNetwork := true
		if _, ok := svccfg.Networks["public"]; ok {
			privateNetwork = false
		}

		ports := convertPorts(svccfg.Ports)
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
			StaticFiles: staticFiles,
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
