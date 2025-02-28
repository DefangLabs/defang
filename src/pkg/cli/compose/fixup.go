package compose

import (
	"context"
	"slices"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
)

// HACK: Use magic network name "public" to determine if the service is public
const NetworkPublic = "public"

func FixupServices(ctx context.Context, provider client.Provider, project *types.Project, upload UploadMode) error {
	// Preload the current config so we can detect which environment variables should be passed as "secrets"
	config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
	if err != nil {
		term.Debugf("failed to load config: %v", err)
		config = &defangv1.Secrets{}
	}
	slices.Sort(config.Names) // sort for binary search

	for _, svccfg := range project.Services {
		// Fixup ports (which affects service name replacement by ReplaceServiceNameWithDNS)
		for i, port := range svccfg.Ports {
			fixupPort(&port)
			svccfg.Ports[i] = port
		}
	}
	svcNameReplacer := NewServiceNameReplacer(provider, project.Services)

	for _, svccfg := range project.Services {
		// Upload the build context, if any; TODO: parallelize
		if svccfg.Build != nil {
			// Pack the build context into a tarball and upload
			url, err := getRemoteBuildContext(ctx, provider, project.Name, svccfg.Name, svccfg.Build, upload)
			if err != nil {
				return err
			}
			svccfg.Build.Context = url

			for key, value := range svccfg.Build.Args {
				if key == "" || value == nil {
					term.Warnf("service %q: skipping unset build argument %q", svccfg.Name, key)
					delete(svccfg.Build.Args, key) // remove the empty key; this is safe
					continue
				}

				val := svcNameReplacer.ReplaceServiceNameWithDNS(svccfg.Name, key, *value, BuildArgs)
				svccfg.Build.Args[key] = &val
			}
		}

		// Fixup secret references; secrets are supposed to be files, not env, but it's kept for backward compatibility
		for i, secret := range svccfg.Secrets {
			if i == 0 { // only warn once
				term.Warnf("service %q: secrets will be exposed as environment variables, not files (use 'environment' instead)", svccfg.Name)
			}
			svccfg.Environment[secret.Source] = nil
		}
		svccfg.Secrets = nil

		// Fixup environment variables
		for key, value := range svccfg.Environment {
			// A bug in Compose-go env file parsing can cause empty keys
			if key == "" {
				term.Warnf("service %q: skipping unset environment variable key", svccfg.Name)
				delete(svccfg.Environment, key) // remove the empty key; this is safe
				continue
			}
			if value == nil {
				continue // will be from config
			}

			// Check if the environment variable is an existing config; if so, mark it as such
			if _, ok := slices.BinarySearch(config.Names, key); ok {
				if svcNameReplacer.HasServiceName(*value) {
					term.Warnf("service %q: environment variable %q will use the `defang config` value instead of adjusted service name", svccfg.Name, key)
				} else {
					term.Warnf("service %q: environment variable %q overridden by config", svccfg.Name, key)
				}
				svccfg.Environment[key] = nil
				continue
			}

			val := svcNameReplacer.ReplaceServiceNameWithDNS(svccfg.Name, key, *value, EnvironmentVars)
			svccfg.Environment[key] = &val
		}

		_, redis := svccfg.Extensions["x-defang-redis"]
		if redis {
			if _, ok := provider.(*client.PlaygroundProvider); ok {
				term.Warnf("service %q: Managed redis is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
				delete(svccfg.Extensions, "x-defang-redis")
			} else if len(svccfg.Ports) == 0 {
				// HACK: we must have at least one host port to get a CNAME for the service https://redis.io/docs/latest/operate/oss_and_stack/management/config/
				var port uint32 = 6379
				// Check entrypoint or command for --port argument
				args := append(svccfg.Entrypoint, svccfg.Command...)
				for i, arg := range args {
					if arg == "--port" {
						if p, err := strconv.ParseUint(args[i+1], 10, 16); err != nil {
							return err
						} else {
							port = uint32(p)
							break
						}
					}
				}
				term.Debugf("service %q: adding redis host port %d", svccfg.Name, port)
				svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
			}
		}

		_, postgres := svccfg.Extensions["x-defang-postgres"]
		if postgres {
			if _, ok := provider.(*client.PlaygroundProvider); ok {
				term.Warnf("service %q: managed postgres is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
				delete(svccfg.Extensions, "x-defang-postgres")
			} else if len(svccfg.Ports) == 0 {
				// HACK: we must have at least one host port to get a CNAME for the service
				var port uint32 = 5432
				// Check PGPORT environment variable for port number https://www.postgresql.org/docs/current/libpq-envars.html
				if pgport := svccfg.Environment["PGPORT"]; pgport != nil {
					if p, err := strconv.ParseUint(*pgport, 10, 16); err != nil {
						return err
					} else {
						port = uint32(p)
					}
				}
				term.Debugf("service %q: adding postgres host port %d", svccfg.Name, port)
				svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
			}
		}

		if !redis && !postgres && isStatefulImage(svccfg.Image) {
			term.Warnf("service %q: stateful service will lose data on restart; use a managed service instead", svccfg.Name)
		}

		// update the concrete service with the fixed up object
		project.Services[svccfg.Name] = svccfg
	}

	return nil
}
