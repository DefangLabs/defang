package compose

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type FixupTarget string

const (
	BuildArgs       FixupTarget = "build argument"
	EnvironmentVars FixupTarget = "environment variable"
)

type ServiceNameReplacer struct {
	dnsResolver           client.DNSResolver
	skipPublicReplacement bool
	projectName           string
	privateServiceNames   *regexp.Regexp
	publicServiceNames    *regexp.Regexp
}

func NewServiceNameReplacer(ctx context.Context, dnsResolver client.DNSResolver, project *composeTypes.Project) ServiceNameReplacer {
	var skipPublicReplacement bool
	if err := dnsResolver.UpdateShardDomain(ctx); err != nil {
		slog.Debug("failed to update shard domain", "error", err)
		skipPublicReplacement = true
	}
	// Create a regexp to detect private service names in environment variable and build arg values
	var privateServiceNames []string // services with private "host" ports
	var publicServiceNames []string  // services with "ingress" ports
	for _, svccfg := range project.Services {
		// HACK: we only check the ports for "host" mode and don't care about the networks; TODO: consider dependsOn / networks
		if hasHostPort(svccfg) {
			privateServiceNames = append(privateServiceNames, regexp.QuoteMeta(svccfg.Name))
		} else if len(svccfg.Ports) > 0 {
			publicServiceNames = append(publicServiceNames, regexp.QuoteMeta(svccfg.Name))
		}
	}

	return ServiceNameReplacer{
		dnsResolver:           dnsResolver,
		projectName:           project.Name,
		privateServiceNames:   makeServiceNameRegex(privateServiceNames),
		publicServiceNames:    makeServiceNameRegex(publicServiceNames),
		skipPublicReplacement: skipPublicReplacement,
	}
}

func (s *ServiceNameReplacer) replaceServiceNameWithDNS(value string) string {
	// First check for private services
	if s.privateServiceNames != nil {
		match := s.privateServiceNames.FindStringSubmatchIndex(value)
		if match != nil {
			// [0] and [1] are the start and end of full match, resp. [2] and [3] are the start and end of the first submatch, etc.
			serviceStart := match[2]
			serviceEnd := match[3]
			serviceName := value[serviceStart:serviceEnd]
			return value[:serviceStart] + s.dnsResolver.ServicePrivateDNS(NormalizeServiceName(serviceName)) + value[serviceEnd:]
		}
	}

	// Then check for public services
	if s.publicServiceNames != nil {
		match := s.publicServiceNames.FindStringSubmatchIndex(value)
		if match != nil {
			serviceStart := match[2]
			serviceEnd := match[3]
			serviceName := value[serviceStart:serviceEnd]
			if s.skipPublicReplacement {
				slog.Warn(fmt.Sprintf("service %q: reference to public DNS cannot be replaced in %q, use `defang login` and try again", serviceName, value))
			} else {
				return value[:serviceStart] + s.dnsResolver.ServicePublicDNS(NormalizeServiceName(serviceName), s.projectName) + value[serviceEnd:]
			}
		}
	}

	return value
}

func (s *ServiceNameReplacer) ReplaceServiceNameWithDNS(serviceName string, key, value string, fixupTarget FixupTarget) string {
	val := s.replaceServiceNameWithDNS(value)

	if val != value {
		slog.Debug("service name was adjusted", "service", serviceName, "fixupTarget", fixupTarget, "key", key, "value", val)
	} else if s.publicServiceNames != nil && s.publicServiceNames.MatchString(value) {
		slog.Debug("service name was not adjusted; only references to other services with port mode set to 'host' will be fixed-up", "service", serviceName, "fixupTarget", fixupTarget, "key", key)
	}

	return val
}

func (s *ServiceNameReplacer) ContainsPrivateServiceName(name string) bool {
	return s.privateServiceNames != nil && s.privateServiceNames.MatchString(name)
}

func isHostPort(port composeTypes.ServicePortConfig) bool {
	return port.Mode == Mode_HOST
}

func hasHostPort(service composeTypes.ServiceConfig) bool {
	return slices.ContainsFunc(service.Ports, isHostPort)
}

func makeServiceNameRegex(quotedServiceNames []string) *regexp.Regexp {
	if len(quotedServiceNames) == 0 {
		return nil
	}
	// This regexp matches service names that are not part of a longer word (e.g. "service1" but not "service1a")
	// and are followed by a slash, a colon+port, or the end of the string.
	return regexp.MustCompile(`\b(` + strings.Join(quotedServiceNames, "|") + `)(?:\/|:\d+|$)`) // first submatch is service name
}
