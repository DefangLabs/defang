package compose

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type serviceNameReplacerMockClient struct {
	client.Client
}

func (m serviceNameReplacerMockClient) ServiceDNS(name string) string {
	return "override-" + name
}

func setup() ServiceNameReplacer {
	services := composeTypes.Services{}
	services["host-serviceA"] = composeTypes.ServiceConfig{
		Name: "host-serviceA",
		Ports: []composeTypes.ServicePortConfig{
			{Mode: "host"},
		},
	}

	services["host-serviceB"] = composeTypes.ServiceConfig{
		Name: "host-serviceB",
		Ports: []composeTypes.ServicePortConfig{
			{Mode: "host"},
		},
	}

	services["ingress-serviceC"] = composeTypes.ServiceConfig{
		Name: "ingress-serviceC",
		Ports: []composeTypes.ServicePortConfig{
			{Mode: "ingress"},
		},
	}

	services["ingress-serviceD"] = composeTypes.ServiceConfig{
		Name: "ingress-serviceD",
		Ports: []composeTypes.ServicePortConfig{
			{Mode: "ingress"},
		},
	}

	return NewServiceNameReplacer(serviceNameReplacerMockClient{}, services)
}

func TestServiceNameReplacer(t *testing.T) {
	testCases := []struct {
		service     string
		key         string
		value       string
		fixUpTarget FixupTarget
		expected    string
	}{
		// host - build args
		{service: "host-serviceA", key: "BuildArg1", value: "value1", fixUpTarget: BuildArgs, expected: "value1"},
		{service: "host-serviceA", key: "BuildArg2", value: "host-serviceB", fixUpTarget: BuildArgs, expected: "override-host-serviceb"},
		{service: "host-serviceA", key: "BuildArg3", value: "ingress-serviceC", fixUpTarget: BuildArgs, expected: "ingress-serviceC"},
		{service: "host-serviceA", key: "BuildArg4", value: "ingress-serviceD", fixUpTarget: BuildArgs, expected: "ingress-serviceD"},

		// host - env args
		{service: "host-serviceA", key: "env1", value: "value1", fixUpTarget: EnvironmentVars, expected: "value1"},
		{service: "host-serviceA", key: "env2", value: "host-serviceB", fixUpTarget: EnvironmentVars, expected: "override-host-serviceb"},
		{service: "host-serviceA", key: "env3", value: "ingress-serviceC", fixUpTarget: EnvironmentVars, expected: "ingress-serviceC"},
		{service: "host-serviceA", key: "env4", value: "ingress-serviceD", fixUpTarget: EnvironmentVars, expected: "ingress-serviceD"},

		// ingress - build args
		{service: "ingress-serviceD", key: "BuildArg1", value: "value1", fixUpTarget: BuildArgs, expected: "value1"},
		{service: "ingress-serviceD", key: "BuildArg2", value: "host-serviceA", fixUpTarget: BuildArgs, expected: "override-host-servicea"},
		{service: "ingress-serviceD", key: "BuildArg3", value: "host-serviceB", fixUpTarget: BuildArgs, expected: "override-host-serviceb"},
		{service: "ingress-serviceD", key: "BuildArg4", value: "ingress-serviceC", fixUpTarget: BuildArgs, expected: "ingress-serviceC"},

		// ingress - env args
		{service: "ingress-serviceD", key: "env1", value: "value1", fixUpTarget: EnvironmentVars, expected: "value1"},
		{service: "ingress-serviceD", key: "env2", value: "host-serviceA", fixUpTarget: EnvironmentVars, expected: "override-host-servicea"},
		{service: "ingress-serviceD", key: "env3", value: "host-serviceB", fixUpTarget: EnvironmentVars, expected: "override-host-serviceb"},
		{service: "ingress-serviceD", key: "env4", value: "ingress-serviceC", fixUpTarget: EnvironmentVars, expected: "ingress-serviceC"},
	}

	// Create a service name replacer
	replacer := setup()

	for _, tc := range testCases {
		got := replacer.ReplaceServiceNameWithDNS(tc.service, tc.key, tc.value, tc.fixUpTarget)
		if got != tc.expected {
			t.Errorf("Expected %q, got %q", tc.expected, got)
		}
	}
}

func TestServiceNameReplacerHasService(t *testing.T) {
	replacer := setup()

	if !replacer.HasServiceName("host-serviceA") {
		t.Error("Expected to have host-serviceA")
	}

	if replacer.HasServiceName("missing-service") {
		t.Error("Expected to not have missing-service")
	}
}
