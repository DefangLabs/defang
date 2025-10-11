package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/modes"
	_type "github.com/DefangLabs/defang/src/protos/google/type"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockEstimateCLI implements EstimateCLIInterface for testing
type MockEstimateCLI struct {
	ConnectError       error
	LoadProjectError   error
	RunEstimateError   error
	EstimateResponse   *defangv1.EstimateResponse
	Project            *compose.Project
	CapturedOutput     string
	CallLog            []string
	ProviderIDAfterSet client.ProviderID // Track the providerID that gets set
}

func (m *MockEstimateCLI) Connect(ctx context.Context, cluster string) (client.FabricClient, error) {
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

func (m *MockEstimateCLI) RunEstimate(ctx context.Context, project *compose.Project, fabric client.FabricClient, provider client.Provider, providerId client.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error) {
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

func (m *MockEstimateCLI) PrintEstimate(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("PrintEstimate(%s)", mode.String()))
}

func (m *MockEstimateCLI) ConfigureLoader(request mcp.CallToolRequest) client.Loader {
	m.CallLog = append(m.CallLog, "ConfigureLoader")
	return nil
}

func (m *MockEstimateCLI) CreatePlaygroundProvider(fabric client.FabricClient) client.Provider {
	m.CallLog = append(m.CallLog, "CreatePlaygroundProvider")
	return nil
}

func (m *MockEstimateCLI) CaptureTermOutput(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) string {
	m.CallLog = append(m.CallLog, fmt.Sprintf("CaptureTermOutput(%s)", mode.String()))
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
			expectedError: "Unknown deployment mode \"UNKNOWN-MODE\", please use one of " + strings.Join(modes.AllDeploymentModes(), ", "),
		},
		{
			name: "load_project_error",
			setupMock: func(m *MockEstimateCLI) {
				m.LoadProjectError = errors.New("failed to parse compose file")
			},
			expectedError: "failed to parse compose file: failed to parse compose file: failed to parse compose file",
		},
		{
			name: "set_provider_id_error",
			arguments: map[string]interface{}{
				"provider": "invalid-provider",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
			},
			expectedError: "Invalid provider specified: provider not one of [auto defang aws digitalocean gcp]",
		},
		{
			name: "run_estimate_error",
			arguments: map[string]interface{}{
				"provider": "aws",
				"region":   "us-west-2",
			},
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.RunEstimateError = errors.New("estimate failed")
			},
			expectedError: "Failed to run estimate: estimate failed",
		},
		{
			name: "successful_estimate_default_mode",
			arguments: map[string]interface{}{
				"provider": "aws",
				"region":   "us-west-2",
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

			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "estimate",
					Arguments: tt.arguments,
				},
			}

			providerID := client.ProviderAuto // Default provider ID

			// Call the function
			loader := &client.MockLoader{}
			params, err := parseEstimateParams(request, &providerID)
			if err != nil {
				// If parsing params fails, check if this was the expected error
				if tt.expectedError != "" {
					assert.EqualError(t, err, tt.expectedError)
					return
				} else {
					assert.NoError(t, err)
				}
			}
			fabric := &MockGrpcClient{}
			result, err := handleEstimateTool(t.Context(), loader, params, fabric, mockCLI)

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
			if tt.expectedError == "" && tt.name == "successful_estimate_default_mode" {
				expectedCalls := []string{
					"LoadProject",
					"CreatePlaygroundProvider",
					"RunEstimate(test-project, aws, DEVELOPMENT)",
					"CaptureTermOutput(DEVELOPMENT)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
