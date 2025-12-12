package tools

import (
	"context"
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
			expectedResult: "Stack &{\"test-stack\" \"aws\" \"us-test-2\" \"\" \"\" \"\"} selected for tool calls.",
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
			expectedResult: "Stack &{\"test-stack\" \"aws\" \"us-test-2\" \"\" \"\" \"\"} selected for tool calls.",
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
		{
			name: "Error when StackConfig.Stack is nil",
			params: SelectStackParams{
				LoaderParams: common.LoaderParams{
					WorkingDirectory: ".",
				},
				Stack: "test-stack",
			},
			initialStack:   nil,
			expectedResult: "",
			expectedError:  true,
			errorContains:  "invariant violated: stack is nil, please restart the MCP server and try again",
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

			result, err = HandleSelectStackTool(context.Background(), tt.params, stackConfig)

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
				// With *sc.Stack = *stack, the content of the stack is copied to the existing pointer
				// So stackConfig.Stack should be modified to have the new stack values
				assert.NotNil(t, stackConfig.Stack)
				assert.Equal(t, "test-stack", stackConfig.Stack.Name)
				assert.Equal(t, client.ProviderAWS, stackConfig.Stack.Provider)
				assert.Equal(t, "us-test-2", stackConfig.Stack.Region)
				assert.Equal(t, os.Getenv("DEFANG_PROVIDER"), "aws")
				assert.Equal(t, os.Getenv("AWS_PROFILE"), "default")
				assert.Equal(t, os.Getenv("AWS_REGION"), "us-test-2")
			}
		})
	}
}

func TestSelectStackParams(t *testing.T) {
	t.Run("Stack parameter validation", func(t *testing.T) {
		params := SelectStackParams{
			LoaderParams: common.LoaderParams{
				WorkingDirectory: ".",
			},
			Stack: "test-stack",
		}

		assert.Equal(t, "test-stack", params.Stack)
		assert.Equal(t, ".", params.WorkingDirectory)
	})

	t.Run("Inherits LoaderParams fields", func(t *testing.T) {
		params := SelectStackParams{
			LoaderParams: common.LoaderParams{
				WorkingDirectory: "/some/path",
				ProjectName:      "my-project",
				ComposeFilePaths: []string{"docker-compose.yml", "docker-compose.override.yml"},
			},
			Stack: "production-stack",
		}

		assert.Equal(t, "production-stack", params.Stack)
		assert.Equal(t, "/some/path", params.WorkingDirectory)
		assert.Equal(t, "my-project", params.ProjectName)
		assert.Equal(t, []string{"docker-compose.yml", "docker-compose.override.yml"}, params.ComposeFilePaths)
	})
}
