package tools

import (
	"context"
	"errors"
	"testing"

	defangcli "github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockCLI implements CLIInterface for testing
type MockCLI struct {
	ConnectError                     error
	NewProviderError                 error
	LoadProjectNameWithFallbackError error
	MockClient                       *client.GrpcClient
	MockProvider                     client.Provider
	MockProjectName                  string

	GetServicesError    error
	MockServices        []deployment_info.Service
	GetServicesCalled   bool
	GetServicesProject  string
	GetServicesProvider client.Provider
}

func (m *MockCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return m.MockClient, nil
}

func (m *MockCLI) NewProvider(ctx context.Context, providerId client.ProviderID, fabricClient client.FabricClient) (client.Provider, error) {
	if m.NewProviderError != nil {
		return nil, m.NewProviderError
	}
	return m.MockProvider, nil
}

func (m *MockCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	if m.LoadProjectNameWithFallbackError != nil {
		return "", m.LoadProjectNameWithFallbackError
	}
	if m.MockProjectName != "" {
		return m.MockProjectName, nil
	}
	return "default-project", nil
}

func (m *MockCLI) GetServices(ctx context.Context, projectName string, provider client.Provider) ([]deployment_info.Service, error) {
	m.GetServicesCalled = true
	m.GetServicesProject = projectName
	m.GetServicesProvider = provider
	if m.GetServicesError != nil {
		return nil, m.GetServicesError
	}
	return m.MockServices, nil
}

func createServicesCallToolRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "services",
			Arguments: args,
		},
	}
}

// createConnectError creates a connect error with the specified code and message
func createConnectError(code connect.Code, message string) error {
	return connect.NewError(code, errors.New(message))
}

func TestHandleServicesToolWithMockCLI(t *testing.T) {
	ctx := t.Context()

	// Common test data
	const (
		testCluster = "test-cluster"
	)

	tests := []struct {
		name                string
		providerId          client.ProviderID
		requestArgs         map[string]interface{}
		mockCLI             *MockCLI
		expectedError       bool
		expectedResultError bool // whether result.IsError should be true
		errorMessage        string
		resultTextContains  string // expected text in result for non-error results
		expectedGetServices bool
		expectedProjectName string
	}{
		// Error cases
		{
			name:       "connect_error",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				ConnectError: errors.New("connection failed"),
			},

			expectedError:       true,
			expectedResultError: true,
			errorMessage:        "connection failed",
			expectedGetServices: false,
		},
		{
			name:       "provider_creation_error",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				NewProviderError: errors.New("provider creation failed"),
			},

			expectedError:       true,
			expectedResultError: true,
			errorMessage:        "provider creation failed",
			expectedGetServices: false,
		},
		{
			name:       "auto_provider_not_configured",
			providerId: client.ProviderAuto,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient: &client.GrpcClient{},
			},

			expectedError:       true,
			expectedResultError: true,
			errorMessage:        "no provider is configured",
			expectedGetServices: false,
		},
		{
			name:       "invalid_working_directory",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": "/nonexistent/directory",
			},
			mockCLI: &MockCLI{
				MockClient: &client.GrpcClient{},
			},

			expectedError:       true,
			expectedResultError: true,
			errorMessage:        "no such file or directory",
			expectedGetServices: false,
		},
		{
			name:       "load_project_name_error",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient:                       &client.GrpcClient{},
				MockProvider:                     &client.PlaygroundProvider{},
				LoadProjectNameWithFallbackError: errors.New("failed to load project name"),
			},

			expectedError:       true,
			expectedResultError: true,
			errorMessage:        "failed to load project name",
			expectedGetServices: false,
		},

		// GetServices error cases - these return different result types
		{
			name:       "get_services_no_services_error",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				MockProvider:     &client.PlaygroundProvider{},
				MockProjectName:  "test-project",
				GetServicesError: defangcli.ErrNoServices{ProjectName: "test-project"},
			},
			expectedError:       true,  // Go error is returned
			expectedResultError: false, // but result.IsError is false (text result)
			errorMessage:        "no services found in project",
			resultTextContains:  "No services found for the specified project test-project",
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},
		{
			name:       "get_services_project_not_deployed",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				MockProvider:     &client.PlaygroundProvider{},
				MockProjectName:  "test-project",
				GetServicesError: createConnectError(connect.CodeNotFound, "project test-project is not deployed in Playground"),
			},
			expectedError:       true,
			expectedResultError: false, // text result, not error result
			errorMessage:        "is not deployed in Playground",
			resultTextContains:  "Project test-project is not deployed in Playground",
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},
		{
			name:       "get_services_generic_error",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				MockProvider:     &client.PlaygroundProvider{},
				MockProjectName:  "test-project",
				GetServicesError: errors.New("generic GetServices failure"),
			},
			expectedError:       false, // Returns text result, no Go error
			expectedResultError: false,
			resultTextContains:  "Failed to get services",
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},

		// Success case
		{
			name:       "successful_cli_operations_until_get_services",
			providerId: client.ProviderDefang,
			requestArgs: map[string]interface{}{
				"working_directory": ".",
			},
			mockCLI: &MockCLI{
				MockClient:      &client.GrpcClient{},
				MockProvider:    &client.PlaygroundProvider{},
				MockProjectName: "test-project",
				MockServices: []deployment_info.Service{
					{
						Service:      "test-service",
						DeploymentId: "test-deployment",
						PublicFqdn:   "test.example.com",
						PrivateFqdn:  "test.internal",
						Status:       "running",
					},
				},
			},
			expectedError:       false,
			expectedResultError: false,
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := createServicesCallToolRequest(tt.requestArgs)
			result, err := handleServicesTool(ctx, request, &tt.providerId, testCluster, tt.mockCLI)

			// Check Go error expectation
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}

			// Check result error expectation
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResultError, result.IsError, "result.IsError expectation")

			// Check result text content for non-error results
			if !tt.expectedResultError && tt.resultTextContains != "" && len(result.Content) > 0 {
				if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
					assert.Contains(t, textContent.Text, tt.resultTextContains)
				}
			}

			// Verify GetServices call expectations
			assert.Equal(t, tt.expectedGetServices, tt.mockCLI.GetServicesCalled, "GetServices call expectation")

			// Verify project name if GetServices was called
			if tt.expectedGetServices && tt.expectedProjectName != "" {
				assert.Equal(t, tt.expectedProjectName, tt.mockCLI.GetServicesProject)
			}
		})
	}
}
