package tools

import (
	"context"
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
					Name:       "test-stack",
					Provider:   client.ProviderAWS,
					Region:     "us-west-2",
					Mode:       modes.ModeAffordable,
					AWSProfile: "default",
				},
			},
			expected:      "This currently selected stack is \"test-stack\": AWS_PROFILE=\"default\"\nAWS_REGION=\"us-west-2\"\nDEFANG_MODE=\"affordable\"\nDEFANG_PROVIDER=\"aws\"",
			expectedError: false,
		},
		{
			name:          "Stack is nil",
			stackConfig:   StackConfig{},
			expected:      "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HandleCurrentStackTool(context.Background(), tt.stackConfig)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
