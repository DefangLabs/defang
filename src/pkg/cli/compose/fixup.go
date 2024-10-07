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

// HACK: Use magic network name "public" to determine if the service is public
const NetworkPublic = "public"

func FixupServices(ctx context.Context, c client.Client, serviceConfigs compose.Services, force BuildContext) error {
	// Create a regexp to detect private service names in environment variable values
	var serviceNames []string
	var nonReplaceServiceNames []string
	for _, svccfg := range serviceConfigs {
		if _, public := svccfg.Networks[NetworkPublic]; !public && slices.ContainsFunc(svccfg.Ports, func(p compose.ServicePortConfig) bool {
			return p.Mode == "host" // only private services with host ports get DNS names
		}) {
			serviceNames = append(serviceNames, regexp.QuoteMeta(svccfg.Name))
		} else {
			nonReplaceServiceNames = append(nonReplaceServiceNames, regexp.QuoteMeta(svccfg.Name))
		}
	}
	var serviceNameRegex *regexp.Regexp
	if len(serviceNames) > 0 {
		serviceNameRegex = regexp.MustCompile(`\b(?:` + strings.Join(serviceNames, "|") + `)\b`)
	}
	var nonReplaceServiceNameRegex *regexp.Regexp
	if len(nonReplaceServiceNames) > 0 {
		nonReplaceServiceNameRegex = regexp.MustCompile(`\b(?:` + strings.Join(nonReplaceServiceNames, "|") + `)\b`)
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
					term.Warnf("service %q: skipping unset build argument %q", svccfg.Name, key)
					delete(svccfg.Build.Args, key) // remove the empty key; this is safe
					continue
				}
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
				if serviceNameRegex != nil && serviceNameRegex.MatchString(*value) {
					term.Warnf("service %q: environment variable %q needs service name fix-up, but is overridden by config, which will not be fixed up.", svccfg.Name, key)
				} else {
					term.Warnf("service %q: environment variable %q overridden by config", svccfg.Name, key)
				}
				svccfg.Environment[key] = nil
				continue
			}

			val := *value
			if serviceNameRegex != nil {
				// Replace service names with their actual DNS names; TODO: support public names too
				val = serviceNameRegex.ReplaceAllStringFunc(*value, func(serviceName string) string {
					return c.ServiceDNS(NormalizeServiceName(serviceName))
				})
				if val != *value {
					term.Warnf("service %q: service names were fixed up in environment variable %q: %q", svccfg.Name, key, val)
				} else if nonReplaceServiceNameRegex != nil && nonReplaceServiceNameRegex.MatchString(*value) {
					term.Warnf("service %q: service names in the environment variable %q were not fixed up, only services with port mode set to host will be fixed up.", svccfg.Name, key)
				}
			}
			svccfg.Environment[key] = &val
		}

		_, redis := svccfg.Extensions["x-defang-redis"]
		_, postgres := svccfg.Extensions["x-defang-postgres"]
		if !redis && !postgres && isStatefulImage(svccfg.Image) {
			term.Warnf("service %q: stateful service will lose data on restart; use a managed service instead", svccfg.Name)
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
