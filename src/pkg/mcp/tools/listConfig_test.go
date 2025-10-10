package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockListConfigCLI implements ListConfigCLIInterface for testing
type MockListConfigCLI struct {
	ConnectError         error
	NewProviderError     error
	LoadProjectNameError error
	ListConfigError      error
	ConfigResponse       *defangv1.Secrets
	ProjectName          string
	CallLog              []string
}

func (m *MockListConfigCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockListConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient) (client.Provider, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("NewProvider(%s)", providerId))
	if m.NewProviderError != nil {
		return nil, m.NewProviderError
	}
	return nil, nil // Mock provider
}

func (m *MockListConfigCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	m.CallLog = append(m.CallLog, "LoadProjectNameWithFallback")
	if m.LoadProjectNameError != nil {
		return "", m.LoadProjectNameError
	}
	return m.ProjectName, nil
}

func (m *MockListConfigCLI) ListConfig(ctx context.Context, provider client.Provider, projectName string) (*defangv1.Secrets, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("ListConfig(%s)", projectName))
	if m.ListConfigError != nil {
		return nil, m.ListConfigError
	}
	return m.ConfigResponse, nil
}

func TestHandleListConfigTool(t *testing.T) {
	tests := []struct {
		name                 string
		providerID           client.ProviderID
		setupMock            func(*MockListConfigCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name:          "provider_auto_not_configured",
			providerID:    client.ProviderAuto,
			setupMock:     func(m *MockListConfigCLI) {},
			expectedError: common.ErrNoProviderSet.Error(),
		},
		{
			name:       "connect_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ConnectError = errors.New("connection failed")
			},
			expectedError: "Could not connect: connection failed",
		},
		{
			name:       "new_provider_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.NewProviderError = errors.New("provider creation failed")
			},
			expectedError: "Failed to get new provider: provider creation failed",
		},
		{
			name:       "load_project_name_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.LoadProjectNameError = errors.New("failed to load project name")
			},
			expectedError: "Failed to load project name: failed to load project name",
		},
		{
			name:       "list_config_error",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ListConfigError = errors.New("failed to list configs")
			},
			expectedError: "Failed to list config variables: failed to list configs",
		},
		{
			name:       "no_config_variables_found",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigResponse = &defangv1.Secrets{
					Names: []string{},
				}
			},
			expectedTextContains: "No config variables found for the project \"test-project\"",
		},
		{
			name:       "successful_list_single_config",
			providerID: client.ProviderAWS,
			setupMock: func(m *MockListConfigCLI) {
				m.ProjectName = "test-project"
				m.ConfigResponse = &defangv1.Secrets{
					Names: []string{"DATABASE_URL"},
				}
			},
			expectedTextContains: "Here is the list of config variables for the project \"test-project\": DATABASE_URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockListConfigCLI{
				CallLog: []string{},
			}
			tt.setupMock(mockCLI)

			// Create request
			args := map[string]interface{}{}

			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "list_configs",
					Arguments: args,
				},
			}

			// Call the function
			loader := &client.MockLoader{}
			result, err := handleListConfigTool(t.Context(), loader, request, &tt.providerID, "test-cluster", mockCLI)

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
			if tt.expectedError == "" && tt.name == "successful_list_single_config" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"NewProvider(aws)",
					"LoadProjectNameWithFallback",
					"ListConfig(test-project)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
