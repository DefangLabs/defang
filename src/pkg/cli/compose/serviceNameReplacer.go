package compose

import (
	"regexp"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	compose "github.com/compose-spec/compose-go/v2/types"
)

type ReplacementMode int

const (
	BuildArgs ReplacementMode = iota
	EnvironmentVars
)

type ServiceNameReplacer struct {
	client                     client.Client
	nonReplaceServiceNameRegex *regexp.Regexp
	serviceNameRegex           *regexp.Regexp
}

func NewServiceNameReplacer(client client.Client, services compose.Services) ServiceNameReplacer {
	// Create a regexp to detect private service names in environment variable and build arg values
	var serviceNames []string
	var nonReplaceServiceNames []string
	for _, svccfg := range services {
		if network(&svccfg) == defangv1.Network_PRIVATE && slices.ContainsFunc(svccfg.Ports, func(p compose.ServicePortConfig) bool {
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
		client:                     client,
		nonReplaceServiceNameRegex: nonReplaceServiceNameRegex,
		serviceNameRegex:           serviceNameRegex,
	}
}

func (s *ServiceNameReplacer) replaceServiceNameWithDNS(serviceName string, key, value string, replacementMode ReplacementMode) string {
	val := value
	if s.serviceNameRegex != nil && s.nonReplaceServiceNameRegex != nil {
		// Replace service names with their actual DNS names; TODO: support public names too
		val = s.serviceNameRegex.ReplaceAllStringFunc(value, func(serviceName string) string {
			return s.client.ServiceDNS(NormalizeServiceName(serviceName))
		})

		fixupTarget := "environment variable"
		if replacementMode == BuildArgs {
			fixupTarget = "build argument"
		}
		if val != value {
			term.Warnf("service %q: service name was fixed up: %s %q assigned value %q", serviceName, fixupTarget, key, val)
		} else if s.nonReplaceServiceNameRegex != nil && s.nonReplaceServiceNameRegex.MatchString(value) {
			term.Warnf("service %q: service name(s) in the %s %q were not adjusted, only references to other services with port mode set to 'host' will be fixed-up", serviceName, fixupTarget, key)
		}
	}

	return val
}

func (s *ServiceNameReplacer) hasServiceName(name string) bool {
	return s.serviceNameRegex != nil && s.serviceNameRegex.MatchString(name)
}
