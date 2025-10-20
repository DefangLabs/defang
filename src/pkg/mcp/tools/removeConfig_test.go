package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockRemoveConfigCLI implements RemoveConfigCLIInterface for testing
type MockRemoveConfigCLI struct {
	ConnectError              error
	NewProviderError          error
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

func (m *MockRemoveConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient) (client.Provider, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	if m.NewProviderError != nil {
		return nil, m.NewProviderError
	}
	return nil, nil // Mock provider
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
		providerID           client.ProviderID
		setupMock            func(*MockRemoveConfigCLI)
		expectError          bool
		expectedTextContains string
		expectedError        string
	}{
		{
			name:          "provider_auto_not_configured",
			configName:    "DATABASE_URL",
			providerID:    client.ProviderAuto,
			setupMock:     func(m *MockRemoveConfigCLI) {},
			expectError:   true,
			expectedError: common.ErrNoProviderSet.Error(),
		},
		{
			name:          "missing_config_name",
			configName:    "",
			providerID:    client.ProviderAWS,
			setupMock:     func(m *MockRemoveConfigCLI) {},
			expectError:   true,
			expectedError: "missing config `name`: required argument \"name\" not found",
		},
		{
			name:       "connect_error",
			configName: "DATABASE_URL",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ConnectError = errors.New("connection failed")
			},
			expectError:   true,
			expectedError: "Could not connect: connection failed",
		},
		{
			name:       "new_provider_error",
			configName: "DATABASE_URL",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockRemoveConfigCLI) {
				m.NewProviderError = errors.New("provider creation failed")
			},
			expectError:   true,
			expectedError: "Failed to get new provider: provider creation failed",
		},
		{
			name:       "load_project_name_error",
			configName: "DATABASE_URL",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockRemoveConfigCLI) {
				m.LoadProjectNameError = errors.New("failed to load project name")
			},
			expectError:   true,
			expectedError: "Failed to load project name: failed to load project name",
		},
		{
			name:       "config_not_found",
			configName: "NONEXISTENT_CONFIG",
			providerID: client.ProviderAWS,
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
			providerID: client.ProviderAWS,
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigDeleteError = errors.New("failed to delete config")
			},
			expectError:   true,
			expectedError: "Failed to remove config variable \"DATABASE_URL\" from project \"test-project\": failed to delete config",
		},
		{
			name:       "successful_config_removal",
			configName: "DATABASE_URL",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockRemoveConfigCLI) {
				m.ProjectName = "test-project"
				// No errors, successful removal
			},
			expectError:          false,
			expectedTextContains: "Successfully remove the config variable \"DATABASE_URL\" from project \"test-project\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "remove_config",
					Arguments: args,
				},
			}

			params, err := parseRemoveConfigParams(request)
			if err != nil {
				if tt.expectError {
					assert.EqualError(t, err, tt.expectedError)
					return
				} else {
					assert.NoError(t, err)
				}
			}

			// Call the function
			loader := &client.MockLoader{}
			result, err := handleRemoveConfigTool(t.Context(), loader, params, &tt.providerID, "test-cluster", mockCLI)

			// Verify error expectations
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.EqualError(t, err, tt.expectedError) // Ensure err is not nil before checking its message
				}
			} else {
				assert.NoError(t, err)
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
