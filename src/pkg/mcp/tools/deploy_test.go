package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockGrpcClient is a mock that implements the Track method safely
type MockGrpcClient struct {
	*client.GrpcClient
}

func (m *MockGrpcClient) Track(event string, props ...interface{}) error {
	// Mock implementation that doesn't panic
	return nil
}

// MockDeployCLI implements DeployCLIInterface for testing
type MockDeployCLI struct {
	ConnectError                 error
	NewProviderError             error
	ComposeUpError               error
	CheckProviderConfiguredError error
	LoadProjectError             error
	OpenBrowserError             error
	ComposeUpResponse            *defangv1.DeployResponse
	Project                      *compose.Project
	CallLog                      []string
}

func (m *MockDeployCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	// Return a base GrpcClient - we need to handle Track method differently
	return &client.GrpcClient{}, nil
}

func (m *MockDeployCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client *client.GrpcClient) (client.Provider, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	return nil, m.NewProviderError
}

func (m *MockDeployCLI) ComposeUp(ctx context.Context, project *compose.Project, grpcClient *client.GrpcClient, provider client.Provider, uploadMode compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *compose.Project, error) {
	m.CallLog = append(m.CallLog, "ComposeUp")
	if m.ComposeUpError != nil {
		return nil, nil, m.ComposeUpError
	}
	return m.ComposeUpResponse, m.Project, nil
}

func (m *MockDeployCLI) CheckProviderConfigured(ctx context.Context, grpcClient *client.GrpcClient, providerId client.ProviderID, projectName string, serviceCount int) (client.Provider, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("CheckProviderConfigured(%s, %s, %d)", providerId, projectName, serviceCount))
	return nil, m.CheckProviderConfiguredError
}

func (m *MockDeployCLI) LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error) {
	m.CallLog = append(m.CallLog, "LoadProject")
	if m.LoadProjectError != nil {
		return nil, m.LoadProjectError
	}
	return m.Project, nil
}

func (m *MockDeployCLI) ConfigureLoader(request mcp.CallToolRequest) client.Loader {
	m.CallLog = append(m.CallLog, "ConfigureLoader")
	return nil
}

func (m *MockDeployCLI) OpenBrowser(url string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("OpenBrowser(%s)", url))
	return m.OpenBrowserError
}

func TestHandleDeployTool(t *testing.T) {
	tests := []struct {
		name                  string
		workingDirectory      string
		providerID            client.ProviderID
		setupMock             func(*MockDeployCLI)
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
			setupMock:             func(m *MockDeployCLI) {},
			expectError:           false, // Function returns but with error result
			expectErrorResult:     true,
			expectedErrorContains: "working_directory is required",
		},
		{
			name:                  "invalid_working_directory",
			workingDirectory:      "/nonexistent/directory", // This will cause os.Chdir to fail in real execution
			providerID:            client.ProviderAWS,
			setupMock:             func(m *MockDeployCLI) {},
			expectError:           true, // os.Chdir will return error and function will return it
			expectErrorResult:     true,
			expectedErrorContains: "no such file or directory", // This is what os.Chdir returns
		},
		{
			name:             "load_project_error",
			workingDirectory: ".", // Use current directory for tests that should proceed past chdir
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.LoadProjectError = errors.New("failed to parse compose file")
			},
			expectError:          true, // LoadProject error returns Go error
			expectTextResult:     true,
			expectedTextContains: "Local deployment failed",
		},
		{
			name:             "connect_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ConnectError = errors.New("connection failed")
			},
			expectError:           true, // Connect error returns Go error
			expectErrorResult:     true,
			expectedErrorContains: "connection failed", // This is the actual error message
		},
		{
			name:             "check_provider_configured_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.CheckProviderConfiguredError = errors.New("provider not configured")
			},
			expectError:           true, // CheckProviderConfigured error returns Go error
			expectErrorResult:     true,
			expectedErrorContains: "provider not configured",
		},
		{
			name:             "compose_up_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpError = errors.New("compose up failed")
			},
			expectError:           true, // ComposeUp error returns Go error
			expectErrorResult:     true,
			expectedErrorContains: "compose up failed",
		},
		{
			name:             "no_services_deployed",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag:     "test-etag",
					Services: []*defangv1.ServiceInfo{}, // Empty services
				}
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Failed to deploy services",
		},
		{
			name:             "successful_deploy_defang_provider",
			workingDirectory: ".", // Use current directory for successful tests
			providerID:       client.ProviderDefang,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag: "test-etag",
					Services: []*defangv1.ServiceInfo{
						{Service: &defangv1.Service{Name: "web"}, PublicFqdn: "web.example.com", Status: "running"},
					},
				}
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Please use the web portal url:",
		},
		{
			name:             "successful_deploy_aws_provider",
			workingDirectory: ".", // Use current directory for successful tests
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag: "test-etag",
					Services: []*defangv1.ServiceInfo{
						{Service: &defangv1.Service{Name: "web"}, PublicFqdn: "web.example.com", Status: "running"},
					},
				}
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Please use the aws console",
		},
		{
			name:                  "provider_auto_not_configured",
			workingDirectory:      ".",
			providerID:            client.ProviderAuto,
			setupMock:             func(m *MockDeployCLI) {},
			expectError:           true,
			expectedErrorContains: "no provider is configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockDeployCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Create request
			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name: "deploy",
					Arguments: map[string]interface{}{
						"working_directory": tt.workingDirectory,
					},
				},
			}

			// Call the function
			result, err := handleDeployTool(t.Context(), request, &tt.providerID, "test-cluster", mockCLI)

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
			if !tt.expectError && tt.workingDirectory != "" && tt.name == "successful_deploy_defang_provider" {
				expectedCalls := []string{
					"ConfigureLoader",
					"LoadProject",
					"Connect(test-cluster)",
					"CheckProviderConfigured(defang, test-project, 0)",
					"ComposeUp",
					// Note: OpenBrowser is called in a goroutine, so it may not be tracked in time
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
