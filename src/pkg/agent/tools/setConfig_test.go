package tools

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockSetConfigCLI implements CLIInterface for testing
type MockSetConfigCLI struct {
	CLIInterface
	ConnectError          error
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

func (m *MockSetConfigCLI) NewProvider(ctx context.Context, providerId client.ProviderID, fabricClient client.FabricClient, stack string) client.Provider {
	m.NewProviderCalled = true
	if m.ReturnedProvider != nil {
		return m.ReturnedProvider
	}
	// Return a simple mock provider to avoid nil pointer issues
	return &MockProvider{}
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

func TestHandleSetConfig(t *testing.T) {
	// Common test data
	const (
		testCluster    = "test-cluster"
		testConfigName = "test-config"
		testValue      = "test-value"
	)
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
			name:                     "missing config name",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"value": testValue},
			mockCLI:                  &MockSetConfigCLI{},
			expectedError:            true,
			errorMessage:             "Invalid config name: secret name \"\" is not valid",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
		},
		{
			name:                     "empty config name",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": "", "value": testValue},
			mockCLI:                  &MockSetConfigCLI{},
			expectedError:            true,
			errorMessage:             "Invalid config name: secret name \"\" is not valid",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
		},
		{
			name:                     "missing config value",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": testConfigName},
			mockCLI:                  &MockSetConfigCLI{},
			expectedError:            true,
			errorMessage:             "Invalid config name: secret name \"test-config\" is not valid",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
		},
		{
			name:                     "empty config value",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": testConfigName, "value": ""},
			mockCLI:                  &MockSetConfigCLI{},
			expectedError:            true,
			errorMessage:             "Invalid config name: secret name \"test-config\" is not valid",
			expectedConnectCalls:     true,
			expectedProviderCalls:    true,
			expectedProjectNameCalls: true,
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
			name:                     "load project name error",
			cluster:                  testCluster,
			providerId:               client.ProviderID(""),
			requestArgs:              map[string]interface{}{"name": testConfigName, "value": testValue},
			mockCLI:                  &MockSetConfigCLI{LoadProjectNameError: errors.New("project loading failed")},
			expectedError:            true,
			errorMessage:             "failed to load project name: project loading failed",
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
			t.Chdir("testdata")
			os.Unsetenv("DEFANG_PROVIDER")
			os.Unsetenv("AWS_PROFILE")
			os.Unsetenv("AWS_REGION")

			loader := &client.MockLoader{}

			// Extract arguments with defaults for missing values
			name := ""
			if n, ok := tt.requestArgs["name"].(string); ok {
				name = n
			}
			value := ""
			if v, ok := tt.requestArgs["value"].(string); ok {
				value = v
			}

			params := SetConfigParams{
				Name:  name,
				Value: value,
			}
			ec := elicitations.NewController(&mockElicitationsClient{
				responses: map[string]string{
					"strategy":     "profile",
					"profile_name": "default",
				},
			})
			stackName := "test-stack"
			result, err := HandleSetConfig(t.Context(), loader, params, tt.mockCLI, ec, StackConfig{
				Cluster:    tt.cluster,
				ProviderID: &tt.providerId,
				Stack:      &stackName,
			})

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.EqualError(t, err, tt.errorMessage)
				}
			} else {
				require.NoError(t, err)
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
