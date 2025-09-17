package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockDestroyCLI implements DestroyCLIInterface for testing
type MockDestroyCLI struct {
	ConnectError                     error
	NewProviderError                 error
	ComposeDownError                 error
	LoadProjectNameWithFallbackError error
	CanIUseProviderError             error
	ComposeDownResult                string
	ProjectName                      string
	CallLog                          []string
}

func (m *MockDestroyCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockDestroyCLI) NewProvider(ctx context.Context, providerId client.ProviderID, grpcClient *client.GrpcClient) (client.Provider, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	return nil, m.NewProviderError
}

func (m *MockDestroyCLI) ComposeDown(ctx context.Context, projectName string, grpcClient *client.GrpcClient, provider client.Provider) (string, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("ComposeDown(%s)", projectName))
	if m.ComposeDownError != nil {
		return "", m.ComposeDownError
	}
	return m.ComposeDownResult, nil
}

func (m *MockDestroyCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	m.CallLog = append(m.CallLog, "LoadProjectNameWithFallback")
	if m.LoadProjectNameWithFallbackError != nil {
		return "", m.LoadProjectNameWithFallbackError
	}
	return m.ProjectName, nil
}

func (m *MockDestroyCLI) CanIUseProvider(ctx context.Context, grpcClient *client.GrpcClient, providerId client.ProviderID, projectName string, provider client.Provider, serviceCount int) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("CanIUseProvider(%s, %s, %d)", providerId, projectName, serviceCount))
	return m.CanIUseProviderError
}

func (m *MockDestroyCLI) ConfigureLoader(request mcp.CallToolRequest) client.Loader {
	m.CallLog = append(m.CallLog, "ConfigureLoader")
	return nil
}

func TestHandleDestroyTool(t *testing.T) {
	tests := []struct {
		name                  string
		workingDirectory      string
		providerID            client.ProviderID
		setupMock             func(*MockDestroyCLI)
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
			setupMock:             func(m *MockDestroyCLI) {},
			expectError:           false,
			expectErrorResult:     true,
			expectedErrorContains: "working_directory is required",
		},
		{
			name:                  "invalid_working_directory",
			workingDirectory:      "/nonexistent/directory",
			providerID:            client.ProviderAWS,
			setupMock:             func(m *MockDestroyCLI) {},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "no such file or directory",
		},
		{
			name:             "connect_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
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
			setupMock: func(m *MockDestroyCLI) {
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
			setupMock: func(m *MockDestroyCLI) {
				m.LoadProjectNameWithFallbackError = errors.New("failed to load project name")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "failed to load project name",
		},
		{
			name:             "can_i_use_provider_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.CanIUseProviderError = errors.New("provider not available")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "provider not available",
		},
		{
			name:             "compose_down_project_not_found",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.ComposeDownError = connect.NewError(connect.CodeNotFound, errors.New("project not found"))
			},
			expectError:          true,
			expectTextResult:     true,
			expectedTextContains: "Project not found, nothing to destroy",
		},
		{
			name:             "compose_down_generic_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.ComposeDownError = errors.New("destroy failed")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "destroy failed",
		},
		{
			name:             "successful_destroy",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.ComposeDownResult = "deployment-123"
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "The project is in the process of being destroyed: test-project",
		},
		{
			name:                  "provider_auto_not_configured",
			workingDirectory:      ".",
			providerID:            client.ProviderAuto,
			setupMock:             func(m *MockDestroyCLI) {},
			expectError:           true,
			expectedErrorContains: "no provider is configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockDestroyCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Create request
			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "destroy",
					Arguments: map[string]interface{}{
						"working_directory": tt.workingDirectory,
					},
				},
			}

			// Call the function
			result, err := handleDestroyTool(context.Background(), request, &tt.providerID, "test-cluster", mockCLI)

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
			if !tt.expectError && tt.workingDirectory != "" && tt.name == "successful_destroy" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"ConfigureLoader",
					"LoadProjectNameWithFallback",
					"CanIUseProvider(aws, test-project, 0)",
					"ComposeDown(test-project)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
