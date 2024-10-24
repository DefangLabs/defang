package compose

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	compose "github.com/compose-spec/compose-go/v2/types"
)

type serviceNameReplacerMockClient struct {
	client.Client
}

func (m serviceNameReplacerMockClient) ServiceDNS(name string) string {
	return "override-" + name
}

func setup() ServiceNameReplacer {
	services := compose.Services{}
	services["host-serviceA"] = compose.ServiceConfig{
		Name: "host-serviceA",
		Ports: []compose.ServicePortConfig{
			{Mode: "host"},
		},
	}

	services["host-serviceB"] = compose.ServiceConfig{
		Name: "host-serviceB",
		Ports: []compose.ServicePortConfig{
			{Mode: "host"},
		},
	}

	services["ingress-serviceC"] = compose.ServiceConfig{
		Name: "ingress-serviceC",
		Ports: []compose.ServicePortConfig{
			{Mode: "ingress"},
		},
	}

	services["ingress-serviceD"] = compose.ServiceConfig{
		Name: "ingress-serviceD",
		Ports: []compose.ServicePortConfig{
			{Mode: "ingress"},
		},
	}

	return NewServiceNameReplacer(serviceNameReplacerMockClient{}, services)
}

func TestServiceNameReplacer(t *testing.T) {
	testCases := []struct {
		service  string
		key      string
		value    string
		mode     ReplacementMode
		expected string
	}{
		// host - build args
		{service: "host-serviceA", key: "BuildArg1", value: "value1", mode: BuildArgs, expected: "value1"},
		{service: "host-serviceA", key: "BuildArg2", value: "host-serviceB", mode: BuildArgs, expected: "override-host-serviceb"},
		{service: "host-serviceA", key: "BuildArg3", value: "ingress-serviceC", mode: BuildArgs, expected: "ingress-serviceC"},
		{service: "host-serviceA", key: "BuildArg4", value: "ingress-serviceD", mode: BuildArgs, expected: "ingress-serviceD"},

		// host - env args
		{service: "host-serviceA", key: "env1", value: "value1", mode: EnvironmentVars, expected: "value1"},
		{service: "host-serviceA", key: "env2", value: "host-serviceB", mode: EnvironmentVars, expected: "override-host-serviceb"},
		{service: "host-serviceA", key: "env3", value: "ingress-serviceC", mode: EnvironmentVars, expected: "ingress-serviceC"},
		{service: "host-serviceA", key: "env4", value: "ingress-serviceD", mode: EnvironmentVars, expected: "ingress-serviceD"},

		// ingress - build args
		{service: "ingress-serviceD", key: "BuildArg1", value: "value1", mode: BuildArgs, expected: "value1"},
		{service: "ingress-serviceD", key: "BuildArg2", value: "host-serviceA", mode: BuildArgs, expected: "override-host-servicea"},
		{service: "ingress-serviceD", key: "BuildArg3", value: "host-serviceB", mode: BuildArgs, expected: "override-host-serviceb"},
		{service: "ingress-serviceD", key: "BuildArg4", value: "ingress-serviceC", mode: BuildArgs, expected: "ingress-serviceC"},

		// ingress - env args
		{service: "ingress-serviceD", key: "env1", value: "value1", mode: EnvironmentVars, expected: "value1"},
		{service: "ingress-serviceD", key: "env2", value: "host-serviceA", mode: EnvironmentVars, expected: "override-host-servicea"},
		{service: "ingress-serviceD", key: "env3", value: "host-serviceB", mode: EnvironmentVars, expected: "override-host-serviceb"},
		{service: "ingress-serviceD", key: "env4", value: "ingress-serviceC", mode: EnvironmentVars, expected: "ingress-serviceC"},
	}

	// Create a service name replacer
	replacer := setup()

	for _, tc := range testCases {
		got := replacer.replaceServiceNameWithDNS(tc.service, tc.key, tc.value, tc.mode)
		if got != tc.expected {
			t.Errorf("Expected %q, got %q", tc.expected, got)
		}
	}
}

func TestServiceNameReplacerHasService(t *testing.T) {
	replacer := setup()

	if !replacer.hasServiceName("host-serviceA") {
		t.Error("Expected to have host-serviceA")
	}

	if replacer.hasServiceName("missing-service") {
		t.Error("Expected to not have missing-service")
	}
}
