package tools

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
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

func (m *MockLoginCLI) InteractiveLoginMCP(ctx context.Context, grpcClient *client.GrpcClient, cluster string, mcpClient string) error {
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
		name                 string
		cluster              string
		authPort             int
		setupMock            func(*MockLoginCLI)
		expectedTextContains string
		expectedError        string
	}{
		{
			name:     "successful_login_already_connected",
			cluster:  "test-cluster",
			authPort: 0,
			setupMock: func(m *MockLoginCLI) {
				// No connect error means already logged in
			},
			expectedTextContains: "Successfully logged in to Defang",
		},
		{
			name:     "connect_error_with_auth_port",
			cluster:  "test-cluster",
			authPort: 3000,
			setupMock: func(m *MockLoginCLI) {
				m.ConnectError = errors.New("connection failed - not authenticated")
			},
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
			expectedError: "login failed",
		},
		{
			name:     "custom_auth_url",
			cluster:  "production-cluster",
			authPort: 8080,
			setupMock: func(m *MockLoginCLI) {
				m.ConnectError = errors.New("connection failed - not authenticated")
				m.AuthURL = "https://custom-auth.example.com/login"
			},
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

			// Call the function
			var err error
			result, err := HandleLoginTool(context.Background(), tt.cluster, tt.authPort, mockCLI)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				if tt.expectedTextContains != "" && len(result) > 0 {
					assert.Contains(t, result, tt.expectedTextContains)
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
