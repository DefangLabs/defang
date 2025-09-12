package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
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
	SetProviderIDError error
	EstimateResponse   *defangv1.EstimateResponse
	Project            *compose.Project
	Region             string
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

func (m *MockEstimateCLI) RunEstimate(ctx context.Context, project *compose.Project, grpcClient *client.GrpcClient, provider client.Provider, providerId client.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error) {
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

func (m *MockEstimateCLI) GetRegion(providerId client.ProviderID) string {
	m.CallLog = append(m.CallLog, fmt.Sprintf("GetRegion(%s)", providerId))
	return m.Region
}

func (m *MockEstimateCLI) CreatePlaygroundProvider(grpcClient *client.GrpcClient) client.Provider {
	m.CallLog = append(m.CallLog, "CreatePlaygroundProvider")
	return nil
}

func (m *MockEstimateCLI) SetProviderID(providerId *client.ProviderID, providerString string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("SetProviderID(%s)", providerString))
	if m.SetProviderIDError != nil {
		return m.SetProviderIDError
	}
	// Simulate the actual setting of the provider ID
	if providerString == "" {
		*providerId = m.ProviderIDAfterSet
	} else if providerString == "AWS" || providerString == "aws" {
		*providerId = client.ProviderAWS
	} else if providerString == "GCP" || providerString == "gcp" {
		*providerId = client.ProviderGCP
	} else {
		*providerId = client.ProviderAuto
	}
	return nil
}

func (m *MockEstimateCLI) CaptureTermOutput(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) string {
	m.CallLog = append(m.CallLog, fmt.Sprintf("CaptureTermOutput(%s)", mode.String()))
	return m.CapturedOutput
}

func TestHandleEstimateTool(t *testing.T) {
	tests := []struct {
		name                  string
		workingDirectory      string
		deploymentMode        string
		provider              string
		providerID            client.ProviderID
		setupMock             func(*MockEstimateCLI)
		expectError           bool
		expectTextResult      bool
		expectErrorResult     bool
		expectedTextContains  string
		expectedErrorContains string
	}{
		{
			name:                  "provider_auto_not_supported",
			workingDirectory:      ".",
			providerID:            client.ProviderAuto,
			setupMock:             func(m *MockEstimateCLI) {},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "estimates are only supported for AWS and GCP",
		},
		{
			name:                  "provider_defang_not_supported",
			workingDirectory:      ".",
			providerID:            client.ProviderDefang,
			setupMock:             func(m *MockEstimateCLI) {},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "estimates are only supported for AWS and GCP",
		},
		{
			name:                  "missing_working_directory",
			workingDirectory:      "",
			providerID:            client.ProviderAWS,
			setupMock:             func(m *MockEstimateCLI) {},
			expectError:           false,
			expectErrorResult:     true,
			expectedErrorContains: "working_directory is required",
		},
		{
			name:                  "invalid_working_directory",
			workingDirectory:      "/nonexistent/directory",
			providerID:            client.ProviderAWS,
			setupMock:             func(m *MockEstimateCLI) {},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "no such file or directory",
		},
		{
			name:             "unknown_deployment_mode_defaults",
			workingDirectory: ".",
			deploymentMode:   "unknown-mode",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.Region = "us-west-2"
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
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Successfully estimated the cost of the project to AWS",
		},
		{
			name:             "load_project_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockEstimateCLI) {
				m.LoadProjectError = errors.New("failed to parse compose file")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "failed to parse compose file",
		},
		{
			name:             "connect_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.ConnectError = errors.New("connection failed")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "connection failed",
		},
		{
			name:             "set_provider_id_error",
			workingDirectory: ".",
			provider:         "invalid-provider",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.SetProviderIDError = errors.New("invalid provider")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "invalid provider",
		},
		{
			name:             "run_estimate_error",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.Region = "us-west-2"
				m.RunEstimateError = errors.New("estimate failed")
			},
			expectError:           true,
			expectErrorResult:     true,
			expectedErrorContains: "estimate failed",
		},
		{
			name:             "successful_estimate_default_mode",
			workingDirectory: ".",
			providerID:       client.ProviderAWS,
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.Region = "us-west-2"
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
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Successfully estimated the cost of the project to AWS",
		},
		{
			name:             "successful_estimate_high_availability_mode",
			workingDirectory: ".",
			deploymentMode:   "HIGH AVAILABILITY",
			provider:         "GCP",
			providerID:       client.ProviderGCP,
			setupMock: func(m *MockEstimateCLI) {
				m.Project = &compose.Project{Name: "test-project"}
				m.Region = "us-central1"
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
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Successfully estimated the cost of the project to AWS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockEstimateCLI{
				CallLog: []string{},
				Region:  "us-west-2", // Default region
			}
			tt.setupMock(mockCLI)

			// Create request
			args := map[string]interface{}{
				"working_directory": tt.workingDirectory,
			}
			if tt.deploymentMode != "" {
				args["deployment_mode"] = tt.deploymentMode
			}
			if tt.provider != "" {
				args["provider"] = tt.provider
			}

			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "estimate",
					Arguments: args,
				},
			}

			// Call the function
			result, err := handleEstimateTool(context.Background(), request, &tt.providerID, "test-cluster", mockCLI)

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
			if !tt.expectError && tt.workingDirectory != "" && tt.name == "successful_estimate_default_mode" {
				expectedCalls := []string{
					"ConfigureLoader",
					"LoadProject",
					"Connect(test-cluster)",
					"CreatePlaygroundProvider",
					"SetProviderID(aws)",
					"GetRegion(aws)",
					"RunEstimate(test-project, aws, DEVELOPMENT)",
					"CaptureTermOutput(DEVELOPMENT)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
