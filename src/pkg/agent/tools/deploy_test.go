package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
	DeployedProject              *compose.Project
	DeployedProjects             []*compose.Project
	ComposeUpErrors              []error
	ConfigSetCalls               []string
	UseRealLoader                bool
	Client                       *client.GrpcClient
	CallLog                      []string
}

func (m *MockDeployCLI) Connect(ctx context.Context, fabricAddr string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", fabricAddr))
	if m.ConnectError != nil {
		return &client.GrpcClient{}, m.ConnectError
	}
	if m.Client != nil {
		return m.Client, nil
	}
	// Return a base GrpcClient - we need to handle Track method differently
	return &client.GrpcClient{}, nil
}

func (m *MockDeployCLI) NewProvider(ctx context.Context, providerId client.ProviderID, fabric client.FabricClient, stack string) client.Provider {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	return client.MockProvider{}
}

func (m *MockDeployCLI) InteractiveLoginMCP(ctx context.Context, fabricAddr string, mcpClient string) error {
	m.CallLog = append(m.CallLog, "InteractiveLoginMCP")
	return m.InteractiveLoginMCPError
}

func (m *MockDeployCLI) ComposeUp(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, stack *stacks.Parameters, params cli.ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
	m.CallLog = append(m.CallLog, "ComposeUp")
	m.DeployedProjects = append(m.DeployedProjects, params.Project)
	if len(m.ComposeUpErrors) > 0 {
		err := m.ComposeUpErrors[0]
		m.ComposeUpErrors = m.ComposeUpErrors[1:]
		return nil, params.Project, err
	}
	if m.ComposeUpError != nil {
		return nil, params.Project, m.ComposeUpError
	}
	m.DeployedProject = params.Project
	return m.ComposeUpResponse, params.Project, nil
}

func (m *MockDeployCLI) LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error) {
	m.CallLog = append(m.CallLog, "LoadProject")
	if m.LoadProjectError != nil {
		return nil, m.LoadProjectError
	}
	if m.UseRealLoader {
		return loader.LoadProject(ctx)
	}
	return m.Project, nil
}

func (m *MockDeployCLI) ConfigSet(ctx context.Context, projectName string, provider client.Provider, name, value string) error {
	m.ConfigSetCalls = append(m.ConfigSetCalls, fmt.Sprintf("%s=%s", name, value))
	return nil
}

func (m *MockDeployCLI) CanIUseProvider(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, projectName string, serviceCount int) error {
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
			stack := stacks.Parameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
			}
			params := DeployParams{
				common.LoaderParams{
					WorkingDirectory: ".",
				},
			}
			result, err := HandleDeployTool(t.Context(), loader, params, mockCLI, ec, StackConfig{
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
			if tt.expectedError == "" && tt.name == "successful_deploy_defang_provider" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"LoadProject",
					"CanIUseProvider",
					"ComposeUp",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}

func TestHandleDeployToolUsesPreselectedContext(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.Mkdir("app", 0o755))
	require.NoError(t, os.WriteFile("app/compose.yaml", []byte(`name: agent-interpolation
services:
  web:
    image: alpine
    environment:
      PROVIDER: ${DEFANG_PROVIDER}
      STACK: ${DEFANG_STACK}
`), 0o644))

	loaderParams := common.LoaderParams{WorkingDirectory: "app"}
	stack := &stacks.Parameters{Name: "production", Provider: client.ProviderAWS}
	loader, err := common.ConfigureAgentLoader(loaderParams, stack)
	require.NoError(t, err)
	mockCLI := &MockDeployCLI{
		UseRealLoader: true,
		ComposeUpResponse: &defangv1.DeployResponse{
			Etag: "test-etag",
			Services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "web"}},
			},
		},
	}

	_, err = HandleDeployTool(t.Context(), loader, DeployParams{LoaderParams: loaderParams}, mockCLI, elicitations.NewController(&mockElicitationsClient{}), StackConfig{
		FabricAddr: "test-cluster",
		Stack:      stack,
	})
	require.NoError(t, err)
	require.NotNil(t, mockCLI.DeployedProject)
	env := mockCLI.DeployedProject.Services["web"].Environment
	require.NotNil(t, env["PROVIDER"])
	require.NotNil(t, env["STACK"])
	assert.Equal(t, "aws", *env["PROVIDER"])
	assert.Equal(t, "production", *env["STACK"])
	assert.Equal(t, 1, strings.Count(strings.Join(mockCLI.CallLog, ","), "LoadProject"))
}

func TestHandleDeployToolSelectsStackBeforeLoadingProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll("app/.defang", 0o755))
	require.NoError(t, os.WriteFile("app/.defang/production", []byte("DEFANG_PROVIDER=aws\nAWS_REGION=us-test-2\n"), 0o644))
	require.NoError(t, os.WriteFile("app/.env.production", []byte("STACK_ENV=selected\n"), 0o644))
	require.NoError(t, os.WriteFile("app/compose.yaml", []byte(`name: agent-interpolation
services:
  web:
    image: alpine
    environment:
      PROVIDER: ${DEFANG_PROVIDER}
      STACK: ${DEFANG_STACK}
      STACK_ENV: ${STACK_ENV}
`), 0o644))
	t.Cleanup(func() {
		os.Unsetenv("DEFANG_PROVIDER")
		os.Unsetenv("DEFANG_STACK")
		os.Unsetenv("AWS_REGION")
	})

	fabric := newStackTestFabric(t)

	loaderParams := common.LoaderParams{
		WorkingDirectory: "app",
		ProjectName:      "agent-interpolation",
	}
	stack := &stacks.Parameters{Provider: client.ProviderAuto}
	loader, err := common.ConfigureAgentLoader(loaderParams, stack)
	require.NoError(t, err)
	mockCLI := &MockDeployCLI{
		Client:        fabric,
		UseRealLoader: true,
		ComposeUpResponse: &defangv1.DeployResponse{
			Etag: "test-etag",
			Services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "web"}},
			},
		},
	}
	ec := elicitations.NewController(&mockElicitationsClient{responses: map[string]string{
		"stack": "production [aws us-test-2]",
	}})

	_, err = HandleDeployTool(t.Context(), loader, DeployParams{LoaderParams: loaderParams}, mockCLI, ec, StackConfig{
		FabricAddr: "test-cluster",
		Stack:      stack,
	})
	require.NoError(t, err)
	assert.Equal(t, "production", stack.Name)
	assert.Equal(t, client.ProviderAWS, stack.Provider)
	require.NotNil(t, mockCLI.DeployedProject)
	env := mockCLI.DeployedProject.Services["web"].Environment
	require.NotNil(t, env["PROVIDER"])
	require.NotNil(t, env["STACK"])
	require.NotNil(t, env["STACK_ENV"])
	assert.Equal(t, "aws", *env["PROVIDER"])
	assert.Equal(t, "production", *env["STACK"])
	assert.Equal(t, "selected", *env["STACK_ENV"])
	assert.Equal(t, []string{
		"Connect(test-cluster)",
		"NewProvider(aws)",
		"LoadProject",
		"CanIUseProvider",
		"ComposeUp",
	}, mockCLI.CallLog)
}

func TestHandleDeployToolReusesSelectedProjectOnMissingConfigRetry(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll("app/.defang", 0o755))
	require.NoError(t, os.WriteFile("app/.defang/production", []byte("DEFANG_PROVIDER=aws\n"), 0o644))
	require.NoError(t, os.WriteFile("app/compose.yaml", []byte(`name: agent-interpolation
services:
  web:
    image: alpine
    environment:
      PROVIDER: ${DEFANG_PROVIDER}
      STACK: ${DEFANG_STACK}
`), 0o644))
	t.Cleanup(func() {
		os.Unsetenv("DEFANG_PROVIDER")
		os.Unsetenv("DEFANG_STACK")
	})

	loaderParams := common.LoaderParams{WorkingDirectory: "app", ProjectName: "agent-interpolation"}
	stack := &stacks.Parameters{Provider: client.ProviderAuto}
	loader, err := common.ConfigureAgentLoader(loaderParams, stack)
	require.NoError(t, err)
	mockCLI := &MockDeployCLI{
		Client:          newStackTestFabric(t),
		UseRealLoader:   true,
		ComposeUpErrors: []error{compose.ErrMissingConfig{"TOKEN"}},
		ComposeUpResponse: &defangv1.DeployResponse{
			Etag: "test-etag",
			Services: []*defangv1.ServiceInfo{
				{Service: &defangv1.Service{Name: "web"}},
			},
		},
	}
	ec := elicitations.NewController(&mockElicitationsClient{responses: map[string]string{
		"stack": "production [aws]",
		"TOKEN": "secret",
	}})

	_, err = HandleDeployTool(t.Context(), loader, DeployParams{LoaderParams: loaderParams}, mockCLI, ec, StackConfig{
		FabricAddr: "test-cluster",
		Stack:      stack,
	})
	require.NoError(t, err)
	require.Len(t, mockCLI.DeployedProjects, 2)
	assert.Same(t, mockCLI.DeployedProjects[0], mockCLI.DeployedProjects[1])
	assert.Equal(t, []string{"TOKEN=secret"}, mockCLI.ConfigSetCalls)
	assert.Equal(t, 2, strings.Count(strings.Join(mockCLI.CallLog, ","), "LoadProject"))
}
