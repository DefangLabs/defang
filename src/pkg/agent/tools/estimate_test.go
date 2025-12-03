package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/modes"
	_type "github.com/DefangLabs/defang/src/protos/google/type"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockEstimateCLI implements CLIInterface for testing
type MockEstimateCLI struct {
	CLIInterface
	ConnectError       error
	LoadProjectError   error
	RunEstimateError   error
	EstimateResponse   *defangv1.EstimateResponse
	Project            *compose.Project
	CapturedOutput     string
	CallLog            []string
	ProviderIDAfterSet client.ProviderID // Track the providerID that gets set
}

func (m *MockEstimateCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockEstimateCLI) LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error) {
	m.CallLog = append(m.CallLog, "LoadProject")
	if m.LoadProjectError != nil {
		return nil, m.LoadProjectError
	}
	return m.Project, nil
}

func (m *MockEstimateCLI) RunEstimate(ctx context.Context, project *compose.Project, grpcClient *client.GrpcClient, provider client.Provider, providerId client.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error) {
	projectName := ""
	if project != nil {
		projectName = project.Name
	}
	m.CallLog = append(m.CallLog, fmt.Sprintf("RunEstimate(%s, %s, %s)", projectName, providerId, mode.String()))
	if m.RunEstimateError != nil {
		return nil, m.RunEstimateError
	}
	return m.EstimateResponse, nil
}

func (m *MockEstimateCLI) CreatePlaygroundProvider(grpcClient *client.GrpcClient) client.Provider {
	m.CallLog = append(m.CallLog, "CreatePlaygroundProvider")
	return nil
}

func (m *MockEstimateCLI) PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse) string {
	m.CallLog = append(m.CallLog, fmt.Sprintf("PrintEstimate(%s)", mode.String()))
	return m.CapturedOutput
}

func TestHandleEstimateTool(t *testing.T) {
	tests := []struct {
		name                 string
		arguments            map[string]interface{}
		setupMock            func(*MockEstimateCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name: "unknown_deployment_mode_fails",
			arguments: map[string]interface{}{
				"provider":        "aws",
				"deployment_mode": "unknown-mode",
				"region":          "us-west-2",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ProviderIDAfterSet = client.ProviderAWS
				m.EstimateResponse = &defangv1.EstimateResponse{
					Subtotal: &_type.Money{
						CurrencyCode: "USD",
						Units:        15,
						Nanos:        0,
					},
				}
				m.CapturedOutput = "Estimated cost: $15.00/month"
			},
			expectedError: "invalid mode: unknown-mode, not one of [AFFORDABLE BALANCED HIGH_AVAILABILITY]",
		},
		{
			name: "load_project_error",
			setupMock: func(m *MockEstimateCLI) {
				m.LoadProjectError = errors.New("failed to parse compose file")
			},
			expectedError: "failed to parse compose file: failed to parse compose file: failed to parse compose file",
		},
		{
			name: "connect_error",
			arguments: map[string]interface{}{
				"provider": "aws",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ConnectError = errors.New("connection failed")
			},
			expectedError: "could not connect: connection failed",
		},
		{
			name: "set_provider_id_error",
			arguments: map[string]interface{}{
				"provider": "invalid-provider",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
			},
			expectedError: "provider not one of [auto defang aws digitalocean gcp]",
		},
		{
			name: "run_estimate_error",
			arguments: map[string]interface{}{
				"provider":        "aws",
				"region":          "us-west-2",
				"deployment_mode": "AFFORDABLE",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.RunEstimateError = errors.New("estimate failed")
			},
			expectedError: "failed to run estimate: estimate failed",
		},
		{
			name: "successful_estimate_default_mode",
			arguments: map[string]interface{}{
				"provider":        "aws",
				"region":          "us-west-2",
				"deployment_mode": "AFFORDABLE",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ProviderIDAfterSet = client.ProviderAWS // Set expected provider ID after SetProviderID call
				m.EstimateResponse = &defangv1.EstimateResponse{
					Subtotal: &_type.Money{
						CurrencyCode: "USD",
						Units:        10,
						Nanos:        0,
					},
				}
				m.CapturedOutput = "Estimated cost: $10.00/month"
			},
			expectedTextContains: "Successfully estimated the cost of the project to AWS",
		},
		{
			name: "successful_estimate_high_availability_mode",
			arguments: map[string]interface{}{
				"deployment_mode": "HIGH_AVAILABILITY",
				"provider":        "GCP",
				"region":          "us-central1",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ProviderIDAfterSet = client.ProviderGCP
				m.EstimateResponse = &defangv1.EstimateResponse{
					Subtotal: &_type.Money{
						CurrencyCode: "USD",
						Units:        50,
						Nanos:        0,
					},
				}
				m.CapturedOutput = "Estimated cost: $50.00/month"
			},
			expectedTextContains: "Successfully estimated the cost of the project to Google Cloud Platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockEstimateCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			providerID := client.ProviderAuto // Default provider ID

			// Extract arguments with defaults for missing values
			provider := ""
			if p, ok := tt.arguments["provider"].(string); ok {
				provider = p
			}
			region := ""
			if r, ok := tt.arguments["region"].(string); ok {
				region = r
			}
			deploymentMode := ""
			if d, ok := tt.arguments["deployment_mode"].(string); ok {
				deploymentMode = d
			}

			params := EstimateParams{
				Provider:       provider,
				Region:         region,
				DeploymentMode: deploymentMode,
			}

			// Call the function
			loader := &client.MockLoader{}
			result, err := HandleEstimateTool(t.Context(), loader, params, mockCLI, StackConfig{
				Cluster:    "test-cluster",
				ProviderID: &providerID,
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
			if tt.expectedError == "" && tt.name == "successful_estimate_default_mode" {
				expectedCalls := []string{
					"LoadProject",
					"Connect(test-cluster)",
					"CreatePlaygroundProvider",
					"RunEstimate(test-project, aws, AFFORDABLE)",
					"PrintEstimate(AFFORDABLE)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
