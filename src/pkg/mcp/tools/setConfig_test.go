package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockSetConfigCLI implements SetConfigCLIInterface for testing
type MockSetConfigCLI struct {
	ConnectError          error
	NewProviderError      error
	LoadProjectNameError  error
	ConfigSetError        error
	ConnectCalled         bool
	NewProviderCalled     bool
	LoadProjectNameCalled bool
	ConfigSetCalled       bool
	ReturnedGrpcClient    *client.GrpcClient
	ReturnedProvider      client.Provider
	ReturnedProjectName   string
	ConfigSetProjectName  string
	ConfigSetProvider     client.Provider
	ConfigSetName         string
	ConfigSetValue        string
}

func (m *MockSetConfigCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.ConnectCalled = true
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	if m.ReturnedGrpcClient == nil {
		// Return a non-nil client to avoid nil pointer issues
		m.ReturnedGrpcClient = &client.GrpcClient{}
	}
	return m.ReturnedGrpcClient, nil
}

func (m *MockSetConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, fabricClient client.FabricClient) (client.Provider, error) {
	m.NewProviderCalled = true
	if m.NewProviderError != nil {
		return nil, m.NewProviderError
	}
	if m.ReturnedProvider != nil {
		return m.ReturnedProvider, nil
	}
	// Return a simple mock provider to avoid nil pointer issues
	return &MockProvider{}, nil
}

// MockProvider implements a minimal subset of client.Provider interface for testing
// We'll just embed a nil pointer and let it panic if methods are called in unexpected contexts
type MockProvider struct {
	client.Provider // Embed the interface to avoid implementing all methods
}

func (p *MockProvider) AccountInfo(context.Context) (*client.AccountInfo, error) {
	return &client.AccountInfo{}, nil
}

func (m *MockSetConfigCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	m.LoadProjectNameCalled = true
	if m.LoadProjectNameError != nil {
		return "", m.LoadProjectNameError
	}
	if m.ReturnedProjectName != "" {
		return m.ReturnedProjectName, nil
	}
	return "mock-project", nil
}

func (m *MockSetConfigCLI) ConfigSet(ctx context.Context, projectName string, provider client.Provider, name, value string) error {
	m.ConfigSetCalled = true
	m.ConfigSetProjectName = projectName
	m.ConfigSetProvider = provider
	m.ConfigSetName = name
	m.ConfigSetValue = value
	return m.ConfigSetError
}

func createCallToolRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "setConfig",
			Arguments: args,
		},
	}
}

func TestHandleSetConfig(t *testing.T) {
	// Common test data
	const (
		testCluster    = "test-cluster"
		testConfigName = "test-config"
		testValue      = "test-value"
	)
	testContext := t.Context()

	tests := []struct {
		name                     string
		cluster                  string
		providerId               client.ProviderID
		requestArgs              map[string]interface{}
		mockCLI                  *MockSetConfigCLI
		expectedError            bool
		errorMessage             string
		expectedProjectName      string
		expectedConnectCalls     bool
		expectedProviderCalls    bool
		expectedProjectNameCalls bool
		expectedConfigSetCalls   bool
	}{
		// Input validation tests
		{
			name:          "missing config name",
			cluster:       testCluster,
			providerId:    client.ProviderID(""),
			requestArgs:   map[string]interface{}{"value": testValue},
			mockCLI:       &MockSetConfigCLI{},
			expectedError: true,
			errorMessage:  "missing 'name' parameter: required argument \"name\" not found",
		},
		{
			name:          "empty config name",
			cluster:       testCluster,
			providerId:    client.ProviderID(""),
			requestArgs:   map[string]interface{}{"name": "", "value": testValue},
			mockCLI:       &MockSetConfigCLI{},
			expectedError: true,
			errorMessage:  "missing 'name' parameter: %!w(<nil>)",
		},
		{
			name:          "missing config value",
			cluster:       testCluster,
			providerId:    client.ProviderID(""),
			requestArgs:   map[string]interface{}{"name": testConfigName},
			mockCLI:       &MockSetConfigCLI{},
			expectedError: true,
			errorMessage:  "missing 'value' parameter: required argument \"value\" not found",
		},
		{
			name:          "empty config value",
			cluster:       testCluster,
			providerId:    client.ProviderID(""),
			requestArgs:   map[string]interface{}{"name": testConfigName, "value": ""},
			mockCLI:       &MockSetConfigCLI{},
			expectedError: true,
			errorMessage:  "missing 'value' parameter: %!w(<nil>)",
		},

		// CLI operation error tests
		{
			name:                 "connect error",
			cluster:              testCluster,
			providerId:           client.ProviderID(""),
			requestArgs:          map[string]interface{}{"name": testConfigName, "value": testValue},
			mockCLI:              &MockSetConfigCLI{ConnectError: errors.New("connection failed")},
			expectedError:        true,
			errorMessage:         "Could not connect: connection failed",
			expectedConnectCalls: true,
		},
		{
			name:                  "provider error",
			cluster:               testCluster,
			providerId:            client.ProviderID(""),
			requestArgs:           map[string]interface{}{"name": testConfigName, "value": testValue},
			mockCLI:               &MockSetConfigCLI{NewProviderError: errors.New("provider initialization failed")},
			expectedError:         true,
			errorMessage:          "Failed to get new provider: provider initialization failed",
			expectedConnectCalls:  true,
			expectedProviderCalls: true,
		},
		{
			name:                     "load project name error",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": testConfigName, "value": testValue},
			mockCLI:                  &MockSetConfigCLI{LoadProjectNameError: errors.New("project loading failed")},
			expectedError:            true,
			errorMessage:             "Failed to load project name: project loading failed",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
		},
		{
			name:                     "config set error",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": "valid_config_name", "value": testValue},
			mockCLI:                  &MockSetConfigCLI{ConfigSetError: errors.New("config set failed")},
			expectedError:            true,
			errorMessage:             "Failed to set config: config set failed",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
			expectedConfigSetCalls:   true,
		},

		// Provider-specific tests
		{
			name:        "provider auto not configured",
			cluster:     testCluster,
			providerId:  client.ProviderAuto,
			requestArgs: map[string]interface{}{"name": testConfigName, "value": testValue},
			mockCLI: &MockSetConfigCLI{
				NewProviderError: errors.New("No provider configured. Use one of these setup tools:\n* /mcp.defang.AWS_Setup\n* /mcp.defang.GCP_Setup\n* /mcp.defang.Playground_Setup"),
			},
			expectedError:         true,
			errorMessage:          "No provider configured: no provider is configured; please type in the chat /defang.AWS_Setup for AWS, /defang.GCP_Setup for GCP, or /defang.Playground_Setup for Playground.",
			expectedConnectCalls:  false, // Early return in providerNotConfiguredError
			expectedProviderCalls: false, // Early return in providerNotConfiguredError
		},

		// Success tests
		{
			name:                     "successful config set",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": "valid_config_name", "value": testValue},
			mockCLI:                  &MockSetConfigCLI{},
			expectedError:            false,
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
			expectedConfigSetCalls:   true,
		},
		{
			name:                     "successful config set with project name",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": "valid_config_name", "value": testValue, "project_name": "test-project"},
			mockCLI:                  &MockSetConfigCLI{ReturnedProjectName: "test-project"},
			expectedError:            false,
			expectedProjectName:      "test-project",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
			expectedConfigSetCalls:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := createCallToolRequest(tt.requestArgs)
			loader := &client.MockLoader{}
			params, err := parseSetConfigParams(request)
			if err != nil {
				if tt.expectedError {
					assert.EqualError(t, err, tt.errorMessage)
					return
				} else {
					assert.NoError(t, err)
				}
			}
			result, err := handleSetConfig(testContext, loader, params, &tt.providerId, tt.cluster, tt.mockCLI)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.EqualError(t, err, tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}

			// Verify expected CLI method calls
			assert.Equal(t, tt.expectedConnectCalls, tt.mockCLI.ConnectCalled, "Connect call expectation")
			assert.Equal(t, tt.expectedProviderCalls, tt.mockCLI.NewProviderCalled, "NewProvider call expectation")
			assert.Equal(t, tt.expectedProjectNameCalls, tt.mockCLI.LoadProjectNameCalled, "LoadProjectName call expectation")
			assert.Equal(t, tt.expectedConfigSetCalls, tt.mockCLI.ConfigSetCalled, "ConfigSet call expectation")

			// Check project name if specified
			if tt.expectedProjectName != "" {
				assert.Equal(t, tt.expectedProjectName, tt.mockCLI.ConfigSetProjectName)
			}

			// Verify config values for successful cases
			if !tt.expectedError && tt.expectedConfigSetCalls {
				if configName, exists := tt.requestArgs["name"]; exists {
					assert.Equal(t, configName, tt.mockCLI.ConfigSetName)
				}
				if configValue, exists := tt.requestArgs["value"]; exists {
					assert.Equal(t, configValue, tt.mockCLI.ConfigSetValue)
				}
			}
		})
	}
}
