package compose

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func FixupServices(ctx context.Context, c client.Client, serviceConfigs compose.Services, force BuildContext) error {
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

	for _, svccfg := range serviceConfigs {
		if svccfg.Deploy != nil {
			if svccfg.Deploy.Replicas == nil {
				one := 1 // default to one replica per service; allow the user to override this to 0
				svccfg.Deploy.Replicas = &one
			}
		}

		// Upload the build context, if any; TODO: parallelize
		if svccfg.Build != nil {
			// Pack the build context into a tarball and upload
			url, err := getRemoteBuildContext(ctx, c, svccfg.Name, svccfg.Build, force)
			if err != nil {
				return err
			}
			svccfg.Build.Context = url

			for key, value := range svccfg.Build.Args {
				if key == "" || value == nil {
					warnf("service %q: skipping unset build argument %q", svccfg.Name, key)
					delete(svccfg.Build.Args, key) // remove the empty key; this is safe
					continue
				}
			}
		}

		// Fixup secret references; secrets are supposed to be files, not env, but it's kept for backward compatibility
		for i, secret := range svccfg.Secrets {
			if i == 0 { // only warn once
				warnf("service %q: secrets will be exposed as environment variables, not files (use 'environment' instead)", svccfg.Name)
			}
			svccfg.Environment[secret.Source] = nil
		}
		svccfg.Secrets = nil

		// Fixup environment variables
		for key, value := range svccfg.Environment {
			// A bug in Compose-go env file parsing can cause empty keys
			if key == "" {
				warnf("service %q: skipping unset environment variable key", svccfg.Name)
				delete(svccfg.Environment, key) // remove the empty key; this is safe
				continue
			}
			if value == nil {
				continue // will be from config
			}

			// Check if the environment variable is an existing config; if so, mark it as such
			if _, ok := slices.BinarySearch(config.Names, key); ok {
				term.Warnf("service %q: environment variable %q overridden by config", svccfg.Name, key)
				svccfg.Environment[key] = nil
				continue
			}

			if serviceNameRegex != nil {
				// Replace service names with their actual DNS names; TODO: support public names too
				val := serviceNameRegex.ReplaceAllStringFunc(*value, func(serviceName string) string {
					return c.ServiceDNS(NormalizeServiceName(serviceName))
				})
				if val != *value {
					warnf("service %q: service names were replaced in environment variable %q: %q", svccfg.Name, key, val)
				}
				svccfg.Environment[key] = &val
			}
		}

		_, redis := svccfg.Extensions["x-defang-redis"]
		if !redis && isStatefulImage(svccfg.Image) {
			warnf("service %q: stateful service will lose data on restart; use a managed service instead", svccfg.Name)
		}

		// Fixup ports
		for i, port := range svccfg.Ports {
			fixupPort(&port)
			svccfg.Ports[i] = port
		}

		oldName := svccfg.Name
		svccfg.Name = NormalizeServiceName(svccfg.Name)
		serviceConfigs[oldName] = svccfg
	}
	return nil
}
