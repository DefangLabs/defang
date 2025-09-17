package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockListConfigCLI implements ListConfigCLIInterface for testing
type MockListConfigCLI struct {
	ConnectError         error
	NewProviderError     error
	LoadProjectNameError error
	ListConfigError      error
	ConfigResponse       *defangv1.Secrets
	ProjectName          string
	CallLog              []string
}

func (m *MockListConfigCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockListConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, grpcClient *client.GrpcClient) (client.Provider, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	if m.NewProviderError != nil {
		return nil, m.NewProviderError
	}
	return nil, nil // Mock provider
}

func (m *MockListConfigCLI) ConfigureLoader(request mcp.CallToolRequest) client.Loader {
	m.CallLog = append(m.CallLog, "ConfigureLoader")
	return nil
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

func TestHandleListConfigTool(t *testing.T) {
	tests := []struct {
		name                  string
		workingDirectory      string
		providerID            client.ProviderID
		setupMock             func(*MockListConfigCLI)
		expectError           bool
		expectTextResult      bool
		expectErrorResult     bool
		expectedTextContains  string
		expectedErrorContains string
	}{
		{
			name:                  "missing_working_directory",
			workingDirectory:      "",
			providerID:            client.ProviderAWS,
			setupMock:             func(m *MockListConfigCLI) {},
			expectError:           false,
			expectErrorResult:     true,
			expectedErrorContains: "working_directory is required",
		},
		{
			name:                  "invalid_working_directory",
			workingDirectory:      "/nonexistent/directory",
			providerID:            client.ProviderAWS,
			setupMock:             func(m *MockListConfigCLI) {},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "no such file or directory",
		},
		{
			name:                  "provider_auto_not_configured",
			workingDirectory:      ".",
			providerID:            client.ProviderAuto,
			setupMock:             func(m *MockListConfigCLI) {},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "no provider is configured",
		},
		{
			name:             "connect_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ConnectError = errors.New("connection failed")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "connection failed",
		},
		{
			name:             "new_provider_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.NewProviderError = errors.New("provider creation failed")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "provider creation failed",
		},
		{
			name:             "load_project_name_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.LoadProjectNameError = errors.New("failed to load project name")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "failed to load project name",
		},
		{
			name:             "list_config_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ListConfigError = errors.New("failed to list configs")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "failed to list configs",
		},
		{
			name:             "no_config_variables_found",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigResponse = &defangv1.Secrets{
					Names: []string{},
				}
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "No config variables found for the project \"test-project\"",
		},
		{
			name:             "successful_list_single_config",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigResponse = &defangv1.Secrets{
					Names: []string{"DATABASE_URL"},
				}
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Here is the list of config variables for the project \"test-project\": DATABASE_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockListConfigCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Create request
			args := map[string]interface{}{
				"working_directory": tt.workingDirectory,
			}

			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "list_configs",
					Arguments: args,
				},
			}

			// Call the function
			result, err := handleListConfigTool(context.Background(), request, &tt.providerID, "test-cluster", mockCLI)

			// Verify error expectations
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify result expectations
			if tt.expectTextResult {
				assert.NotNil(t, result)
				assert.NotNil(t, result.Content)
				if tt.expectedTextContains != "" && len(result.Content) > 0 {
					if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
						assert.Contains(t, textContent.Text, tt.expectedTextContains)
					}
				}
			}

			if tt.expectErrorResult {
				assert.NotNil(t, result)
				assert.NotNil(t, result.Content)
				assert.True(t, result.IsError)
				if tt.expectedErrorContains != "" && len(result.Content) > 0 {
					if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
						assert.Contains(t, textContent.Text, tt.expectedErrorContains)
					}
				}
			}

			// For successful cases, verify CLI methods were called in order
			if !tt.expectError && tt.workingDirectory != "" && tt.name == "successful_list_single_config" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"ConfigureLoader",
					"LoadProjectNameWithFallback",
					"ListConfig(test-project)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
