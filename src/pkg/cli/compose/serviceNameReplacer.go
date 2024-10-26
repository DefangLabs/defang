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
	provider                   client.Provider
	nonReplaceServiceNameRegex *regexp.Regexp
	serviceNameRegex           *regexp.Regexp
}

func NewServiceNameReplacer(provider client.Provider, services composeTypes.Services) ServiceNameReplacer {
	// Create a regexp to detect private service names in environment variable and build arg values
	var serviceNames []string
	var nonReplaceServiceNames []string
	for _, svccfg := range services {
		if _, public := svccfg.Networks["public"]; !public && slices.ContainsFunc(svccfg.Ports, func(p composeTypes.ServicePortConfig) bool {
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

	return ServiceNameReplacer{
		provider:                   provider,
		nonReplaceServiceNameRegex: nonReplaceServiceNameRegex,
		serviceNameRegex:           serviceNameRegex,
	}
}

func (s *ServiceNameReplacer) ReplaceServiceNameWithDNS(serviceName string, key, value string, fixupTarget FixupTarget) string {
	val := value
	if s.serviceNameRegex != nil && s.nonReplaceServiceNameRegex != nil {
		// Replace service names with their actual DNS names; TODO: support public names too
		val = s.serviceNameRegex.ReplaceAllStringFunc(value, func(serviceName string) string {
			return s.provider.ServiceDNS(NormalizeServiceName(serviceName))
		})

		if val != value {
			term.Warnf("service %q: service name was adjusted: %s %q assigned value %q", serviceName, fixupTarget, key, val)
		} else if s.nonReplaceServiceNameRegex != nil && s.nonReplaceServiceNameRegex.MatchString(value) {
			term.Warnf("service %q: service name in the %s %q was not adjusted, only references to other services with port mode set to 'host' will be fixed-up", serviceName, fixupTarget, key)
		}
	}

	return val
}

func (s *ServiceNameReplacer) HasServiceName(name string) bool {
	return s.serviceNameRegex != nil && s.serviceNameRegex.MatchString(name)
}
