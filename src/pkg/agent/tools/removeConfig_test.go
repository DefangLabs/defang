package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/bufbuild/connect-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRemoveConfigCLI implements CLIInterface for testing
type MockRemoveConfigCLI struct {
	CLIInterface
	ConnectError              error
	LoadProjectNameError      error
	ConfigDeleteError         error
	ConfigDeleteNotFoundError bool
	ProjectName               string
	CallLog                   []string
}

func (m *MockRemoveConfigCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockRemoveConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient, stack string) client.Provider {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	return nil // Mock provider
}

func (m *MockRemoveConfigCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	m.CallLog = append(m.CallLog, "LoadProjectNameWithFallback")
	if m.LoadProjectNameError != nil {
		return "", m.LoadProjectNameError
	}
	return m.ProjectName, nil
}

func (m *MockRemoveConfigCLI) ConfigDelete(ctx context.Context, projectName string, provider client.Provider, name string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("ConfigDelete(%s, %s)", projectName, name))
	if m.ConfigDeleteNotFoundError {
		return connect.NewError(connect.CodeNotFound, errors.New("config not found"))
	}
	return m.ConfigDeleteError
}

func TestHandleRemoveConfigTool(t *testing.T) {
	tests := []struct {
		name                 string
		configName           string
		setupMock            func(*MockRemoveConfigCLI)
		expectError          bool
		expectedTextContains string
		expectedError        string
	}{
		{
			name:       "connect_error",
			configName: "DATABASE_URL",
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				m.ConnectError = errors.New("connection failed")
			},
			expectError:   true,
			expectedError: "Could not connect: connection failed",
		},
		{
			name:       "load_project_name_error",
			configName: "DATABASE_URL",
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				m.LoadProjectNameError = errors.New("failed to load project name")
			},
			expectError:   true,
			expectedError: "failed to load project name: failed to load project name",
		},
		{
			name:       "config_not_found",
			configName: "NONEXISTENT_CONFIG",
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigDeleteNotFoundError = true
			},
			expectError:          false,
			expectedTextContains: "Config variable \"NONEXISTENT_CONFIG\" not found in project \"test-project\"",
		},
		{
			name:       "config_delete_error",
			configName: "DATABASE_URL",
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigDeleteError = errors.New("failed to delete config")
			},
			expectError:   true,
			expectedError: "failed to remove config variable \"DATABASE_URL\" from project \"test-project\": failed to delete config",
		},
		{
			name:       "successful_config_removal",
			configName: "DATABASE_URL",
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				// No errors, successful removal
			},
			expectError:          false,
			expectedTextContains: "Successfully removed the config variable \"DATABASE_URL\" from project \"test-project\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("testdata")
			os.Unsetenv("DEFANG_PROVIDER")
			os.Unsetenv("AWS_PROFILE")
			os.Unsetenv("AWS_REGION")

			// Create mock and configure it
			mockCLI := &MockRemoveConfigCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Create request
			args := map[string]interface{}{}
			if tt.configName != "" {
				args["name"] = tt.configName
			}

			params := RemoveConfigParams{
				Name: tt.configName,
			}

			// Call the function
			loader := &client.MockLoader{
				Project: compose.Project{Name: "test-project"},
			}
			ec := elicitations.NewController(&mockElicitationsClient{
				responses: map[string]string{
					"strategy":     "profile",
					"profile_name": "default",
				},
			})
			provider := client.ProviderAWS
			stack := stacks.StackParameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
			}
			result, err := HandleRemoveConfigTool(t.Context(), loader, params, mockCLI, ec, StackConfig{
				Cluster:    "test-cluster",
				ProviderID: &provider,
				Stack:      &stack,
			})

			// Verify error expectations
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.EqualError(t, err, tt.expectedError) // Ensure err is not nil before checking its message
				}
			} else {
				require.NoError(t, err)
				if tt.expectedTextContains != "" && len(result) > 0 {
					assert.Contains(t, result, tt.expectedTextContains)
				}
			}

			// For successful cases, verify CLI methods were called in order
			if !tt.expectError && tt.name == "successful_config_removal" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"LoadProjectNameWithFallback",
					"ConfigDelete(test-project, DATABASE_URL)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
