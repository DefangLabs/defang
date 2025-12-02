package tools

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	defangcli "github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/modes"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockCLI implements CLIInterface for testing
type MockCLI struct {
	CLIInterface
	ConnectError                     error
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

func (m *MockCLI) NewProvider(ctx context.Context, providerId client.ProviderID, fabricClient client.FabricClient, stack string) client.Provider {
	return m.MockProvider
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

func (m *MockCLI) ComposeDown(ctx context.Context, projectName string, client *client.GrpcClient, provider client.Provider) (string, error) {
	return "", nil
}

func (m *MockCLI) ComposeUp(ctx context.Context, client *client.GrpcClient, provider client.Provider, params defangcli.ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
	return nil, nil, nil
}

func (m *MockCLI) ConfigDelete(ctx context.Context, projectName string, provider client.Provider, name string) error {
	return nil
}

func (m *MockCLI) ConfigSet(ctx context.Context, projectName string, provider client.Provider, name, value string) error {
	return nil
}

func (m *MockCLI) CreatePlaygroundProvider(client *client.GrpcClient) client.Provider {
	return m.MockProvider
}

func (m *MockCLI) GenerateAuthURL(authPort int) string {
	return ""
}

func (m *MockCLI) InteractiveLoginMCP(ctx context.Context, client *client.GrpcClient, cluster string, mcpClient string) error {
	return nil
}

func (m *MockCLI) ListConfig(ctx context.Context, provider client.Provider, projectName string) (*defangv1.Secrets, error) {
	return nil, nil
}

func (m *MockCLI) LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error) {
	return nil, nil
}

func (m *MockCLI) PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse) string {
	return ""
}

func (m *MockCLI) RunEstimate(ctx context.Context, project *compose.Project, client *client.GrpcClient, provider client.Provider, providerId client.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error) {
	return nil, nil
}

func (m *MockCLI) Tail(ctx context.Context, provider client.Provider, projectName string, options defangcli.TailOptions) error {
	return nil
}

func (m *MockCLI) TailAndMonitor(ctx context.Context, project *compose.Project, provider client.Provider, waitTimeout time.Duration, options defangcli.TailOptions) (defangcli.ServiceStates, error) {
	return nil, nil
}

// createConnectError creates a connect error with the specified code and message
func createConnectError(code connect.Code, message string) error {
	return connect.NewError(code, errors.New(message))
}

type mockElicitationsClient struct {
	responses map[string]string
}

func (m *mockElicitationsClient) Request(ctx context.Context, req elicitations.Request) (elicitations.Response, error) {
	properties, ok := req.Schema["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		panic("invalid schema properties")
	}
	fields := make([]string, 0)
	for field := range properties {
		fields = append(fields, field)
	}

	if len(fields) > 1 {
		panic("mockElicitationsClient only supports single-field requests")
	}

	return elicitations.Response{
		Action: "accept",
		Content: map[string]any{
			fields[0]: m.responses[fields[0]],
		},
	}, nil
}

func TestHandleServicesToolWithMockCLI(t *testing.T) {
	tests := []struct {
		name                string
		providerId          client.ProviderID
		requestArgs         map[string]interface{}
		mockCLI             *MockCLI
		expectedError       bool
		errorMessage        string
		resultTextContains  string // expected text in result for non-error results
		expectedGetServices bool
		expectedProjectName string
	}{
		// Error cases
		{
			name:       "connect_error",
			providerId: client.ProviderDefang,
			mockCLI: &MockCLI{
				ConnectError: errors.New("connection failed"),
			},

			expectedError:       true,
			errorMessage:        "connection failed",
			expectedGetServices: false,
		},
		{
			name:       "load_project_name_error",
			providerId: client.ProviderDefang,
			mockCLI: &MockCLI{
				MockClient:                       &client.GrpcClient{},
				MockProvider:                     &client.PlaygroundProvider{},
				LoadProjectNameWithFallbackError: errors.New("failed to load project name"),
			},

			expectedError:       true,
			errorMessage:        "failed to load project name",
			expectedGetServices: false,
		},

		// GetServices error cases - these return different result types
		{
			name:       "get_services_no_services_error",
			providerId: client.ProviderDefang,
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				MockProvider:     &client.PlaygroundProvider{},
				MockProjectName:  "test-project",
				GetServicesError: defangcli.ErrNoServices{ProjectName: "test-project"},
			},
			expectedError:       false, // Returns successful result with message
			resultTextContains:  "no services found for the specified project",
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},
		{
			name:       "get_services_project_not_deployed",
			providerId: client.ProviderDefang,
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				MockProvider:     &client.PlaygroundProvider{},
				MockProjectName:  "test-project",
				GetServicesError: createConnectError(connect.CodeNotFound, "project test-project is not deployed in Playground"),
			},
			expectedError:       false, // Returns successful result with message
			resultTextContains:  "is not deployed in Playground",
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},
		{
			name:       "get_services_generic_error",
			providerId: client.ProviderDefang,
			mockCLI: &MockCLI{
				MockClient:       &client.GrpcClient{},
				MockProvider:     &client.PlaygroundProvider{},
				MockProjectName:  "test-project",
				GetServicesError: errors.New("generic GetServices failure"),
			},
			expectedError:       true, // Returns text result, no Go error
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},

		// Success case
		{
			name:       "successful_cli_operations_until_get_services",
			providerId: client.ProviderDefang,
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
			expectedGetServices: true,
			expectedProjectName: "test-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("testdata")
			os.Unsetenv("DEFANG_PROVIDER")
			os.Unsetenv("AWS_PROFILE")
			os.Unsetenv("AWS_REGION")
			loader := &client.MockLoader{}
			ec := elicitations.NewController(&mockElicitationsClient{
				responses: map[string]string{
					"strategy":     "profile",
					"profile_name": "default",
				},
			})
			result, err := HandleServicesTool(t.Context(), loader, tt.mockCLI, ec, StackConfig{
				Cluster:    "test-cluster",
				ProviderID: &tt.providerId,
				Stack:      "test-stack",
			})

			// Check Go error expectation
			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
				// Check result text content for non-error results
				if tt.resultTextContains != "" && len(result) > 0 {
					assert.Contains(t, result, tt.resultTextContains)
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
