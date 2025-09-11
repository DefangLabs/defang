package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

func createCallToolRequest(args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "setConfig",
			Arguments: args,
		},
	}
}

func TestHandleSetConfig(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Common test data - extracted to reduce duplication
	const (
		testCluster    = "test-cluster"
		testConfigName = "test-config"
		testValue      = "test-value"
	)
	testProviderId := client.ProviderID("")
	testContext := context.Background()

	tests := []struct {
		name          string
		request       mcp.CallToolRequest
		expectedError bool
		errorMessage  string
	}{
		{
			name: "missing working directory",
			request: createCallToolRequest(map[string]interface{}{
				"name":  testConfigName,
				"value": testValue,
			}),
			expectedError: true,
			errorMessage:  "Invalid working directory",
		},
		{
			name: "empty working directory",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": "",
				"name":              testConfigName,
				"value":             testValue,
			}),
			expectedError: true,
			errorMessage:  "working_directory is required",
		},
		{
			name: "invalid working directory",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": "/nonexistent/directory",
				"name":              testConfigName,
				"value":             testValue,
			}),
			expectedError: true,
			errorMessage:  "Failed to change working directory",
		},
		{
			name: "missing config name",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": tempDir,
				"value":             testValue,
			}),
			expectedError: true,
			errorMessage:  "Invalid config `name`",
		},
		{
			name: "empty config name",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": tempDir,
				"name":              "",
				"value":             testValue,
			}),
			expectedError: true,
			errorMessage:  "`name` is required",
		},
		{
			name: "missing config value",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": tempDir,
				"name":              testConfigName,
			}),
			expectedError: true,
			errorMessage:  "Invalid config `value`",
		},
		{
			name: "empty config value",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": tempDir,
				"name":              testConfigName,
				"value":             "",
			}),
			expectedError: true,
			errorMessage:  "`value` is required",
		},
		{
			name: "successful config set (fails at cluster connection)",
			request: createCallToolRequest(map[string]interface{}{
				"working_directory": tempDir,
				"name":              testConfigName,
				"value":             testValue,
			}),
			expectedError: true,
			errorMessage:  "dial tcp: lookup test-cluster: no such host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := handleSetConfig(testContext, tt.request, testCluster, testProviderId)

			if tt.expectedError {
				assert.NotNil(t, result)
				// For validation errors, the function returns a result with IsError=true, not a Go error
				if result != nil && result.IsError {
					assert.True(t, result.IsError)
					if tt.errorMessage != "" && len(result.Content) > 0 {
						if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
							assert.Contains(t, textContent.Text, tt.errorMessage)
						}
					}
				} else if err != nil {
					// For system errors (like network), we get a Go error
					assert.Error(t, err)
					if tt.errorMessage != "" {
						assert.Contains(t, err.Error(), tt.errorMessage)
					}
				} else {
					t.Errorf("Expected error but got neither result.IsError=true nor Go error")
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.False(t, result.IsError)
			}
		})
	}
}

func TestHandleSetConfigValidWorkingDirectory(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Use common test data
	const (
		testCluster    = "test-cluster"
		testConfigName = "valid-config"
		testValue      = "valid-value"
	)
	testProviderId := client.ProviderID("")

	request := createCallToolRequest(map[string]interface{}{
		"working_directory": tempDir,
		"name":              "test-key",
		"value":             "test-value",
	})
	ctx := context.Background()
	result, err := handleSetConfig(ctx, request, testCluster, testProviderId)

	// Should fail at provider configuration check, but working directory change should succeed
	assert.Error(t, err)
	assert.NotNil(t, result)

	// Verify we're now in the temp directory (resolve symlinks for macOS)
	currentDir, _ := os.Getwd()
	expectedDir, _ := filepath.EvalSymlinks(tempDir)
	actualDir, _ := filepath.EvalSymlinks(currentDir)
	assert.Equal(t, expectedDir, actualDir)
}
