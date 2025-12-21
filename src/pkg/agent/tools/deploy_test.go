package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockGrpcClient is a mock that implements the Track method safely
type MockGrpcClient struct {
	*client.GrpcClient
}

func (m *MockGrpcClient) Track(event string, props ...interface{}) error {
	// Mock implementation that doesn't panic
	return nil
}

// MockDeployCLI implements CLIInterface for testing
type MockDeployCLI struct {
	CLIInterface
	ConnectError                 error
	ComposeUpError               error
	CheckProviderConfiguredError error
	LoadProjectError             error
	OpenBrowserError             error
	InteractiveLoginMCPError     error
	ComposeUpResponse            *defangv1.DeployResponse
	Project                      *compose.Project
	CallLog                      []string
}

func (m *MockDeployCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return &client.GrpcClient{}, m.ConnectError
	}
	// Return a base GrpcClient - we need to handle Track method differently
	return &client.GrpcClient{}, nil
}

func (m *MockDeployCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient, stack string) client.Provider {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	return nil
}

func (m *MockDeployCLI) InteractiveLoginMCP(ctx context.Context, fabric *client.GrpcClient, cluster string, mcpClient string) error {
	m.CallLog = append(m.CallLog, "InteractiveLoginMCP")
	return m.InteractiveLoginMCPError
}

func (m *MockDeployCLI) ComposeUp(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, params cli.ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
	m.CallLog = append(m.CallLog, "ComposeUp")
	if m.ComposeUpError != nil {
		return nil, nil, m.ComposeUpError
	}
	return m.ComposeUpResponse, m.Project, nil
}

func (m *MockDeployCLI) LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error) {
	m.CallLog = append(m.CallLog, "LoadProject")
	if m.LoadProjectError != nil {
		return nil, m.LoadProjectError
	}
	return m.Project, nil
}

func (m *MockDeployCLI) CanIUseProvider(ctx context.Context, fabric *client.GrpcClient, projectName, stackName string, provider client.Provider, serviceCount int) error {
	m.CallLog = append(m.CallLog, "CanIUseProvider")
	return nil
}

func TestHandleDeployTool(t *testing.T) {
	tests := []struct {
		name                 string
		setupMock            func(*MockDeployCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name: "load_project_error",
			setupMock: func(m *MockDeployCLI) {
				m.LoadProjectError = errors.New("failed to parse compose file")
			},
			expectedError: "local deployment failed: failed to parse compose file: failed to parse compose file. Please provide a valid compose file path.",
		},
		{
			name: "connect_error",
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ConnectError = errors.New("connection failed")
				m.InteractiveLoginMCPError = errors.New("connection failed")
			},
			expectedError: "connection failed",
		},
		{
			name: "compose_up_error",
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpError = errors.New("compose up failed")
			},
			expectedError: "failed to compose up services: compose up failed",
		},
		{
			name: "no_services_deployed",
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
			name: "successful_deploy_defang_provider",
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag: "test-etag",
					Services: []*defangv1.ServiceInfo{
						{Service: &defangv1.Service{Name: "web"}, PublicFqdn: "web.example.com", Status: "running"},
					},
				}
			},
			expectedTextContains: "The deployment is not complete, but it has been started successfully",
		},
		{
			name: "successful_deploy_aws_provider",
			setupMock: func(m *MockDeployCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ComposeUpResponse = &defangv1.DeployResponse{
					Etag: "test-etag",
					Services: []*defangv1.ServiceInfo{
						{Service: &defangv1.Service{Name: "web"}, PublicFqdn: "web.example.com", Status: "running"},
					},
				}
			},
			expectedTextContains: "The deployment is not complete, but it has been started successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("testdata")
			os.Unsetenv("DEFANG_PROVIDER")
			os.Unsetenv("AWS_PROFILE")
			os.Unsetenv("AWS_REGION")
			// Create mock and configure it
			mockCLI := &MockDeployCLI{
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
			stack := stacks.StackParameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
			}
			params := DeployParams{
				common.LoaderParams{
					WorkingDirectory: ".",
				},
			}
			result, err := HandleDeployTool(t.Context(), loader, params, mockCLI, ec, StackConfig{
				Cluster: "test-cluster",
				Stack:   &stack,
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
			if tt.expectedError == "" && tt.name == "successful_deploy_defang_provider" {
				expectedCalls := []string{
					"LoadProject",
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"CanIUseProvider",
					"ComposeUp",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
