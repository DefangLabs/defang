package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockListConfigCLI implements CLIInterface for testing
type MockListConfigCLI struct {
	CLIInterface
	ConnectError             error
	LoadProjectNameError     error
	ListConfigError          error
	InteractiveLoginMCPError error
	ConfigResponse           *defangv1.Secrets
	ProjectName              string
	CallLog                  []string
}

func (m *MockListConfigCLI) Connect(ctx context.Context, fabricAddr string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", fabricAddr))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockListConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient, stack string) client.Provider {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	return nil // Mock provider
}

func (m *MockListConfigCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	m.CallLog = append(m.CallLog, "LoadProjectNameWithFallback")
	if m.LoadProjectNameError != nil {
		return "", m.LoadProjectNameError
	}
	return m.ProjectName, nil
}

func (m *MockListConfigCLI) ListConfig(ctx context.Context, provider client.Provider, projectName string) (*defangv1.Secrets, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("ListConfig(%s)", projectName))
	if m.ListConfigError != nil {
		return nil, m.ListConfigError
	}
	return m.ConfigResponse, nil
}

func (m *MockListConfigCLI) InteractiveLoginMCP(ctx context.Context, fabricAddr string, mcpClient string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("InteractiveLoginMCP(%s)", fabricAddr))
	if m.InteractiveLoginMCPError != nil {
		return m.InteractiveLoginMCPError
	}
	return nil
}

func TestHandleListConfigTool(t *testing.T) {
	tests := []struct {
		name                 string
		providerID           client.ProviderID
		setupMock            func(*MockListConfigCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name:       "connect_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ConnectError = errors.New("connection failed")
				m.InteractiveLoginMCPError = errors.New("connection failed")
			},
			expectedError: "connection failed",
		},
		{
			name:       "load_project_name_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.LoadProjectNameError = errors.New("failed to load project name")
			},
			expectedError: "failed to load project name: failed to load project name",
		},
		{
			name:       "list_config_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ListConfigError = errors.New("failed to list configs")
			},
			expectedError: "failed to list config variables: failed to list configs",
		},
		{
			name:       "no_config_variables_found",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigResponse = &defangv1.Secrets{
					Names: []string{},
				}
			},
			expectedTextContains: "No config variables found for the project \"test-project\"",
		},
		{
			name:       "successful_list_single_config",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigResponse = &defangv1.Secrets{
					Names: []string{"DATABASE_URL"},
				}
			},
			expectedTextContains: "Here is the list of config variables for the project \"test-project\": DATABASE_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("testdata")
			os.Unsetenv("DEFANG_PROVIDER")
			os.Unsetenv("AWS_PROFILE")
			os.Unsetenv("AWS_REGION")

			// Create mock and configure it
			mockCLI := &MockListConfigCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Call the function
			loader := &client.MockLoader{}
			ec := elicitations.NewController(&mockElicitationsClient{
				responses: map[string]string{
					"strategy":     "profile",
					"profile_name": "default",
				},
			})

			stack := stacks.Parameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
			}
			params := ListConfigParams{
				LoaderParams: common.LoaderParams{
					WorkingDirectory: ".",
				},
			}
			result, err := HandleListConfigTool(t.Context(), loader, params, mockCLI, ec, StackConfig{
				FabricAddr: "test-cluster",
				Stack:      &stack,
			})

			// Verify error expectations
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
				if tt.expectedTextContains != "" && len(result) > 0 {
					assert.Contains(t, result, tt.expectedTextContains)
				}
			}

			// For successful cases, verify CLI methods were called in order
			if tt.expectedError == "" && tt.name == "successful_list_single_config" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"LoadProjectNameWithFallback",
					"ListConfig(test-project)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
