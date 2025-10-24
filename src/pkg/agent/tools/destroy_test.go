package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/bufbuild/connect-go"
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

func (m *MockDestroyCLI) NewProvider(ctx context.Context, providerId client.ProviderID, grpcClient client.FabricClient) (client.Provider, error) {
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

func TestHandleDestroyTool(t *testing.T) {
	tests := []struct {
		name                 string
		providerID           client.ProviderID
		setupMock            func(*MockDestroyCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name:       "connect_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ConnectError = errors.New("connection failed")
			},
			expectedError: "Could not connect: connection failed",
		},
		{
			name:       "new_provider_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.NewProviderError = errors.New("provider creation failed")
			},
			expectedError: "Failed to get new provider: provider creation failed",
		},
		{
			name:       "load_project_name_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.LoadProjectNameWithFallbackError = errors.New("failed to load project name")
			},
			expectedError: "Failed to load project name: failed to load project name",
		},
		{
			name:       "can_i_use_provider_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.CanIUseProviderError = errors.New("provider not available")
			},
			expectedError: "Failed to use provider: provider not available",
		},
		{
			name:       "compose_down_project_not_found",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.ComposeDownError = connect.NewError(connect.CodeNotFound, errors.New("project not found"))
			},
			expectedError: "Project not found, nothing to destroy. Please use a valid project name, compose file path or project directory.",
		},
		{
			name:       "compose_down_generic_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.ComposeDownError = errors.New("destroy failed")
			},
			expectedError: "Failed to send destroy request: destroy failed",
		},
		{
			name:       "successful_destroy",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDestroyCLI) {
				m.ProjectName = "test-project"
				m.ComposeDownResult = "deployment-123"
			},
			expectedTextContains: "The project is in the process of being destroyed: test-project",
		},
		{
			name:          "provider_auto_not_configured",
			providerID:    client.ProviderAuto,
			setupMock:     func(m *MockDestroyCLI) {},
			expectedError: "no provider is configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockDestroyCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Call the function
			loader := &client.MockLoader{}
			result, err := HandleDestroyTool(t.Context(), loader, &tt.providerID, "test-cluster", mockCLI)

			// Verify error expectations
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				if tt.expectedTextContains != "" && len(result) > 0 {
					assert.Contains(t, result, tt.expectedTextContains)
				}
			}

			// For successful cases, verify CLI methods were called in order
			if tt.expectedError == "" && tt.name == "successful_destroy" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"LoadProjectNameWithFallback",
					"CanIUseProvider(aws, test-project, 0)",
					"ComposeDown(test-project)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
