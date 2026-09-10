package tools

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/stretchr/testify/assert"
)

func toolNames(t *testing.T, ec elicitations.Controller) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, tool := range CollectDefangTools(ec, StackConfig{}) {
		names[tool.Name()] = true
	}
	return names
}

func TestCollectDefangTools_OmitsMutatingToolsWhenUnsupported(t *testing.T) {
	ec := elicitations.NewController(&mockElicitationsClient{})
	ec.SetSupported(false)

	names := toolNames(t, ec)

	for _, mutating := range []string{"deploy", "destroy", "set_config", "remove_config"} {
		assert.False(t, names[mutating], "expected %q to be omitted when elicitation is unsupported", mutating)
	}
	for _, readOnly := range []string{"services", "logs", "estimate", "list_configs", "current_stack"} {
		assert.True(t, names[readOnly], "expected %q to still be offered when elicitation is unsupported", readOnly)
	}
}

func TestCollectDefangTools_IncludesMutatingToolsWhenSupported(t *testing.T) {
	ec := elicitations.NewController(&mockElicitationsClient{})
	ec.SetSupported(true)

	names := toolNames(t, ec)

	for _, mutating := range []string{"deploy", "destroy", "set_config", "remove_config"} {
		assert.True(t, names[mutating], "expected %q to be offered when elicitation is supported", mutating)
	}
}
