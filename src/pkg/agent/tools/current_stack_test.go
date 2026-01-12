package tools

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
)

func TestHandleCurrentStackTool(t *testing.T) {
	tests := []struct {
		name          string
		stackConfig   StackConfig
		expected      string
		expectedError bool
	}{
		{
			name: "Stack is set",
			stackConfig: StackConfig{
				Stack: &stacks.StackParameters{
					Name:     "test-stack",
					Provider: client.ProviderAWS,
					Region:   "us-test-2",
					Mode:     modes.ModeAffordable,
					Variables: map[string]string{
						"AWS_PROFILE": "default",
					},
				},
			},
			expected:      "This currently selected stack is \"test-stack\": AWS_PROFILE=\"default\"\nAWS_REGION=\"us-test-2\"\nDEFANG_MODE=\"affordable\"\nDEFANG_PROVIDER=\"aws\"",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HandleCurrentStackTool(t.Context(), tt.stackConfig)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
