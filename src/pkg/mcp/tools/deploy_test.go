package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

func (m *MockDeployCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient) (client.Provider, error) {
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

func (m *MockDeployCLI) OpenBrowser(url string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("OpenBrowser(%s)", url))
	return m.OpenBrowserError
}

func TestHandleDeployTool(t *testing.T) {
	tests := []struct {
		name                 string
		providerID           client.ProviderID
		setupMock            func(*MockDeployCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name:       "load_project_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.LoadProjectError = errors.New("failed to parse compose file")
			},
			expectedError: "local deployment failed: failed to parse compose file: failed to parse compose file. Please provide a valid compose file path.",
		},
		{
			name:       "connect_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ConnectError = errors.New("connection failed")
			},
			expectedError: "could not connect: connection failed",
		},
		{
			name:       "check_provider_configured_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.CheckProviderConfiguredError = errors.New("provider not configured")
			},
			expectedError: "provider not configured correctly: provider not configured",
		},
		{
			name:       "compose_up_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpError = errors.New("compose up failed")
			},
			expectedError: "failed to compose up services: compose up failed",
		},
		{
			name:       "no_services_deployed",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag:     "test-etag",
					Services: []*defangv1.ServiceInfo{},
				}
			},
			expectedError: "no services deployed",
		},
		{
			name:       "successful_deploy_defang_provider",
			providerID: client.ProviderDefang,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag: "test-etag",
					Services: []*defangv1.ServiceInfo{
						{Service: &defangv1.Service{Name: "web"}, PublicFqdn: "web.example.com", Status: "running"},
					},
				}
			},
			expectedTextContains: "Please use the web portal url:",
		},
		{
			name:       "successful_deploy_aws_provider",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag: "test-etag",
					Services: []*defangv1.ServiceInfo{
						{Service: &defangv1.Service{Name: "web"}, PublicFqdn: "web.example.com", Status: "running"},
					},
				}
			},
			expectedTextContains: "Please use the aws console",
		},
		{
			name:          "provider_auto_not_configured",
			providerID:    client.ProviderAuto,
			setupMock:     func(m *MockDeployCLI) {},
			expectedError: "no provider configured: no provider is configured; please type in the chat /defang.AWS_Setup for AWS, /defang.GCP_Setup for GCP, or /defang.Playground_Setup for Playground.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockDeployCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Call the function
			loader := &client.MockLoader{}
			result, err := handleDeployTool(t.Context(), loader, &tt.providerID, "test-cluster", mockCLI)

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
			if tt.expectedError == "" && tt.name == "successful_deploy_defang_provider" {
				expectedCalls := []string{
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
