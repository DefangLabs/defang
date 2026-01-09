package tools

import (
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
)

func TestHandleSelectStackTool(t *testing.T) {
	tests := []struct {
		name           string
		params         SelectStackParams
		initialStack   *stacks.StackParameters
		expectedResult string
		expectedError  bool
		errorContains  string
	}{
		{
			name: "Successfully select existing test-stack",
			params: SelectStackParams{
				LoaderParams: common.LoaderParams{
					WorkingDirectory: ".",
				},
				Stack: "test-stack",
			},
			initialStack: &stacks.StackParameters{
				Name:     "placeholder",
				Provider: client.ProviderGCP,
				Region:   "placeholder",
				Mode:     modes.ModeAffordable,
			},
			expectedResult: "Stack \"test-stack\" selected.",
			expectedError:  false,
		},
		{
			name: "Replace existing stack with test-stack",
			params: SelectStackParams{
				LoaderParams: common.LoaderParams{
					WorkingDirectory: ".",
				},
				Stack: "test-stack",
			},
			initialStack: &stacks.StackParameters{
				Name:         "old-stack",
				Provider:     client.ProviderGCP,
				Region:       "us-central1",
				Mode:         modes.ModeAffordable,
				GCPProjectID: "old-project",
			},
			expectedResult: "Stack \"test-stack\" selected.",
			expectedError:  false,
		},
		{
			name: "Error when stack does not exist",
			params: SelectStackParams{
				LoaderParams: common.LoaderParams{
					WorkingDirectory: ".",
				},
				Stack: "nonexistent-stack",
			},
			initialStack: &stacks.StackParameters{
				Name: "placeholder",
			},
			expectedResult: "",
			expectedError:  true,
			errorContains:  "Unable to load stack \"nonexistent-stack\"",
		},
		{
			name: "Error with empty stack name",
			params: SelectStackParams{
				LoaderParams: common.LoaderParams{
					WorkingDirectory: ".",
				},
				Stack: "",
			},
			initialStack: &stacks.StackParameters{
				Name: "placeholder",
			},
			expectedResult: "",
			expectedError:  true,
			errorContains:  "Unable to load stack \"\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("testdata")
			os.Unsetenv("DEFANG_PROVIDER")
			os.Unsetenv("AWS_PROFILE")
			os.Unsetenv("AWS_REGION")

			// Initialize stack config with initial stack if provided
			stackConfig := StackConfig{
				Stack: tt.initialStack,
			}

			// Call the function under test
			var result string
			var err error

			result, err = HandleSelectStackTool(t.Context(), tt.params, stackConfig)

			// Verify results
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
				assert.NotNil(t, stackConfig.Stack)
				assert.Equal(t, "test-stack", stackConfig.Stack.Name)
				assert.Equal(t, client.ProviderAWS, stackConfig.Stack.Provider)
				assert.Equal(t, "us-test-2", stackConfig.Stack.Region)
				assert.Equal(t, "aws", os.Getenv("DEFANG_PROVIDER"))
				assert.Equal(t, "default", os.Getenv("AWS_PROFILE"))
				assert.Equal(t, "us-test-2", os.Getenv("AWS_REGION"))
			}
		})
	}
}
