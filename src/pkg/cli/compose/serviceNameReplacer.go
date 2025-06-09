package compose

import (
	"regexp"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type FixupTarget string

const (
	BuildArgs       FixupTarget = "build argument"
	EnvironmentVars FixupTarget = "environment variable"
)

type ServiceNameReplacer struct {
	provider                client.Provider
	hostServiceNames        *regexp.Regexp
	ingressServiceNameRegex *regexp.Regexp
}

func NewServiceNameReplacer(provider client.Provider, project *composeTypes.Project) ServiceNameReplacer {
	// Create a regexp to detect private service names in environment variable and build arg values
	var hostServiceNames []string    // services with private "host" ports
	var ingressServiceNames []string // services with "ingress" ports
	for _, svccfg := range project.Services {
		// HACK: we only check the ports for "host" mode and don't care about the networks
		if slices.ContainsFunc(svccfg.Ports, isHostPort) {
			hostServiceNames = append(hostServiceNames, regexp.QuoteMeta(svccfg.Name))
		} else if len(svccfg.Ports) > 0 {
			ingressServiceNames = append(ingressServiceNames, regexp.QuoteMeta(svccfg.Name))
		}
	}

	return ServiceNameReplacer{
		provider:                provider,
		hostServiceNames:        makeServiceNameRegex(hostServiceNames),
		ingressServiceNameRegex: makeServiceNameRegex(ingressServiceNames),
	}
}

func (s *ServiceNameReplacer) replaceServiceNameWithDNS(value string) string {
	if s.hostServiceNames == nil {
		return value
	}
	match := s.hostServiceNames.FindStringSubmatchIndex(value)
	if match == nil {
		return value
	}
	// [0] and [1] are the start and end of full match, resp. [2] and [3] are the start and end of the first submatch, etc.
	serviceStart := match[2]
	serviceEnd := match[3]
	return value[:serviceStart] + s.provider.ServiceDNS(NormalizeServiceName(value[serviceStart:serviceEnd])) + value[serviceEnd:]
}

func (s *ServiceNameReplacer) ReplaceServiceNameWithDNS(serviceName string, key, value string, fixupTarget FixupTarget) string {
	val := s.replaceServiceNameWithDNS(value)

	if val != value {
		term.Warnf("service %q: service name was adjusted: %s %q assigned value %q", serviceName, fixupTarget, key, val)
	} else if s.ingressServiceNameRegex != nil && s.ingressServiceNameRegex.MatchString(value) {
		term.Debugf("service %q: service name in the %s %q was not adjusted; only references to other services with port mode set to 'host' will be fixed-up", serviceName, fixupTarget, key)
	}

	return val
}

func (s *ServiceNameReplacer) HasServiceName(name string) bool {
	return s.hostServiceNames != nil && s.hostServiceNames.MatchString(name)
}

func isHostPort(port composeTypes.ServicePortConfig) bool {
	return port.Mode == Mode_HOST
}

func makeServiceNameRegex(quotedServiceNames []string) *regexp.Regexp {
	if len(quotedServiceNames) == 0 {
		return nil
	}
	// This regexp matches service names that are not part of a longer word (e.g. "service1" but not "service1a")
	// and are followed by a slash, a colon+port, or the end of the string.
	return regexp.MustCompile(`\b(` + strings.Join(quotedServiceNames, "|") + `)(?:\/|:\d+|$)`) // first submatch is service name
}
