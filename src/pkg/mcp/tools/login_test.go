package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// MockLoginCLI implements LoginCLIInterface for testing
type MockLoginCLI struct {
	ConnectError          error
	InteractiveLoginError error
	AuthURL               string
	CallLog               []string
}

func (m *MockLoginCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	m.CallLog = append(m.CallLog, fmt.Sprintf("Connect(%s)", cluster))
	if m.ConnectError != nil {
		return nil, m.ConnectError
	}
	return &client.GrpcClient{}, nil
}

func (m *MockLoginCLI) InteractiveLoginMCP(ctx context.Context, grpcClient *client.GrpcClient, cluster string) error {
	m.CallLog = append(m.CallLog, fmt.Sprintf("InteractiveLoginMCP(%s)", cluster))
	return m.InteractiveLoginError
}

func (m *MockLoginCLI) GenerateAuthURL(authPort int) string {
	m.CallLog = append(m.CallLog, fmt.Sprintf("GenerateAuthURL(%d)", authPort))
	if m.AuthURL != "" {
		return m.AuthURL
	}
	return fmt.Sprintf("Please open this URL in your browser: http://127.0.0.1:%d to login", authPort)
}

func TestHandleLoginTool(t *testing.T) {
	tests := []struct {
		name                  string
		cluster               string
		authPort              int
		setupMock             func(*MockLoginCLI)
		expectError           bool
		expectTextResult      bool
		expectErrorResult     bool
		expectedTextContains  string
		expectedErrorContains string
	}{
		{
			name:     "successful_login_already_connected",
			cluster:  "test-cluster",
			authPort: 0,
			setupMock: func(m *MockLoginCLI) {
				// No connect error means already logged in
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Successfully logged in to Defang",
		},
		{
			name:     "connect_error_with_auth_port",
			cluster:  "test-cluster",
			authPort: 3000,
			setupMock: func(m *MockLoginCLI) {
				m.ConnectError = errors.New("connection failed - not authenticated")
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Please open this URL in your browser: http://127.0.0.1:3000 to login",
		},
		{
			name:     "connect_error_interactive_login_success",
			cluster:  "test-cluster",
			authPort: 0,
			setupMock: func(m *MockLoginCLI) {
				m.ConnectError = errors.New("connection failed - not authenticated")
				// InteractiveLoginError is nil, so login succeeds
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "Successfully logged in to Defang",
		},
		{
			name:     "connect_error_interactive_login_failure",
			cluster:  "test-cluster",
			authPort: 0,
			setupMock: func(m *MockLoginCLI) {
				m.ConnectError = errors.New("connection failed - not authenticated")
				m.InteractiveLoginError = errors.New("login failed")
			},
			expectError:           false,
			expectErrorResult:     true,
			expectedErrorContains: "login failed",
		},
		{
			name:     "custom_auth_url",
			cluster:  "production-cluster",
			authPort: 8080,
			setupMock: func(m *MockLoginCLI) {
				m.ConnectError = errors.New("connection failed - not authenticated")
				m.AuthURL = "https://custom-auth.example.com/login"
			},
			expectError:          false,
			expectTextResult:     true,
			expectedTextContains: "https://custom-auth.example.com/login",
		},
		{
			// Note: Removed cluster-specific duplicate scenarios (playground/aws) to keep suite concise
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock and configure it
			mockCLI := &MockLoginCLI{CallLog: []string{}}
			if tt.setupMock != nil {
				tt.setupMock(mockCLI)
			}

			// Create request (login tool doesn't require any parameters)
			request := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Name:      "login",
					Arguments: map[string]interface{}{},
				},
			}

			// Call the function
			result, err := handleLoginTool(context.Background(), request, tt.cluster, tt.authPort, mockCLI)

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

			// For specific cases, verify CLI methods were called in order
			if tt.name == "successful_login_already_connected" {
				expectedCalls := []string{
					"Connect(test-cluster)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}

			if tt.name == "connect_error_with_auth_port" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"GenerateAuthURL(3000)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}

			if tt.name == "connect_error_interactive_login_success" {
				expectedCalls := []string{
					"Connect(test-cluster)",
					"InteractiveLoginMCP(test-cluster)",
				}
				assert.Equal(t, expectedCalls, mockCLI.CallLog)
			}
		})
	}
}
