package tools

import (
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
)

func TestCreateStack(t *testing.T) {
	tests := []struct {
		name           string
		params         createStackParams
		expectedResult string
		expectedError  bool
		errorContains  string
	}{
		{
			name: "Successfully create AWS stack with default mode",
			params: createStackParams{
				Name:      "myawsstack",
				Region:    "us-west-2",
				Provider:  client.ProviderAWS,
				Variables: map[string]string{"AWS_PROFILE": "default"},
			},
			expectedResult: "Successfully created stack \"myawsstack\" and loaded its environment.",
			expectedError:  false,
		},
		{
			name: "Successfully create GCP stack with explicit mode",
			params: createStackParams{
				Name:      "mygcpstack",
				Region:    "us-central1",
				Provider:  client.ProviderGCP,
				Mode:      "balanced",
				Variables: map[string]string{"GCP_PROJECT_ID": "my-project"},
			},
			expectedResult: "Successfully created stack \"mygcpstack\" and loaded its environment.",
			expectedError:  false,
		},
		{
			name: "Error with invalid mode",
			params: createStackParams{
				Name:      "invalidstack",
				Region:    "us-west-2",
				Provider:  client.ProviderAWS,
				Mode:      "invalid-mode",
				Variables: map[string]string{"AWS_PROFILE": "default"},
			},
			expectedResult: "Invalid mode provided",
			expectedError:  true,
			errorContains:  "invalid mode",
		},
		{
			name: "Error with empty stack name",
			params: createStackParams{
				Name:      "",
				Region:    "us-west-2",
				Provider:  client.ProviderAWS,
				Variables: map[string]string{"AWS_PROFILE": "default"},
			},
			expectedResult: "Failed to create stack",
			expectedError:  true,
			errorContains:  "stack name cannot be empty",
		},
		{
			name: "Error with invalid stack name",
			params: createStackParams{
				Name:      "Invalid-Name",
				Region:    "us-west-2",
				Provider:  client.ProviderAWS,
				Variables: map[string]string{"AWS_PROFILE": "default"},
			},
			expectedResult: "Failed to create stack",
			expectedError:  true,
			errorContains:  "stack name must start with a letter",
		},
	}
	// unset before running tests
	envVars := []string{
		"DEFANG_PROVIDER",
		"AWS_PROFILE",
		"AWS_REGION",
		"GCP_PROJECT_ID",
		"CLOUDSDK_COMPUTE_REGION",
		"DEFANG_MODE",
		"DEFANG_STACK",
	}
	for _, ev := range envVars {
		// if it was set, restore it after the test
		if val, exists := os.LookupEnv(ev); exists {
			defer t.Setenv(ev, val)
		}
		os.Unsetenv(ev)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			tt.params.WorkingDirectory = tmpDir

			initialStack := &stacks.Parameters{
				Name: "placeholder",
			}
			stackConfig := StackConfig{
				Stack: initialStack,
			}

			result, err := createStack(tt.params, stackConfig)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Equal(t, tt.expectedResult, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
				assert.NotNil(t, stackConfig.Stack)
				assert.Equal(t, tt.params.Name, stackConfig.Stack.Name)
				assert.Equal(t, tt.params.Provider, stackConfig.Stack.Provider)
				assert.Equal(t, tt.params.Region, stackConfig.Stack.Region)
				assert.Equal(t, tt.params.Variables, stackConfig.Stack.Variables)
			}
		})
	}
}
