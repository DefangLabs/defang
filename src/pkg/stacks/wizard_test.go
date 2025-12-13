package stacks

import (
	"context"
	"errors"
	"os"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

// mockElicitationsController is a mock implementation of elicitations.Controller
type mockElicitationsController struct {
	responses        map[string]string
	enumResponses    map[string]string
	defaultResponses map[string]string
	errors           map[string]error
	enumErrors       map[string]error
	defaultErrors    map[string]error
	callOrder        []string
	supported        bool
}

func newMockElicitationsController() *mockElicitationsController {
	return &mockElicitationsController{
		responses:        make(map[string]string),
		enumResponses:    make(map[string]string),
		defaultResponses: make(map[string]string),
		errors:           make(map[string]error),
		enumErrors:       make(map[string]error),
		defaultErrors:    make(map[string]error),
		supported:        true,
	}
}

func (m *mockElicitationsController) RequestString(ctx context.Context, message, field string) (string, error) {
	m.callOrder = append(m.callOrder, "RequestString:"+field)
	if err, exists := m.errors[field]; exists {
		return "", err
	}
	if response, exists := m.responses[field]; exists {
		return response, nil
	}
	return "", errors.New("mock: no response configured for field " + field)
}

func (m *mockElicitationsController) RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error) {
	m.callOrder = append(m.callOrder, "RequestStringWithDefault:"+field)
	if err, exists := m.defaultErrors[field]; exists {
		return "", err
	}
	if response, exists := m.defaultResponses[field]; exists {
		return response, nil
	}
	return defaultValue, nil
}

func (m *mockElicitationsController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	m.callOrder = append(m.callOrder, "RequestEnum:"+field)
	if err, exists := m.enumErrors[field]; exists {
		return "", err
	}
	if response, exists := m.enumResponses[field]; exists {
		return response, nil
	}
	return "", errors.New("mock: no enum response configured for field " + field)
}

func (m *mockElicitationsController) SetSupported(supported bool) {
	m.supported = supported
}

func (m *mockElicitationsController) IsSupported() bool {
	return m.supported
}

func TestNewWizard(t *testing.T) {
	mockController := newMockElicitationsController()
	wizard := NewWizard(mockController)

	if wizard == nil {
		t.Fatal("NewWizard returned nil")
	}
	if wizard.ec != mockController {
		t.Error("NewWizard did not set controller correctly")
	}
}

func TestWizardCollectParameters(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(*mockElicitationsController)
		setupEnv          func(*testing.T)
		cleanupEnv        func()
		expectError       bool
		expectedResult    *StackParameters
		expectedCallOrder []string
	}{
		{
			name: "AWS provider with AWS_PROFILE env var",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultResponses["region"] = "us-east-1"
				m.defaultResponses["stack_name"] = "awsuseast1"
				m.defaultResponses["aws_profile"] = "test-profile"
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("AWS_PROFILE", "test-profile")
			},
			cleanupEnv: func() {
				os.Unsetenv("AWS_PROFILE")
			},
			expectError: false,
			expectedResult: &StackParameters{
				Provider:   cliClient.ProviderAWS,
				Region:     "us-east-1",
				Name:       "awsuseast1",
				AWSProfile: "test-profile",
			},
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
				"RequestStringWithDefault:aws_profile",
			},
		},
		{
			name: "AWS provider without AWS_PROFILE env var",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultResponses["region"] = "us-west-2"
				m.defaultResponses["stack_name"] = "awsuswest2"
				m.enumResponses["aws_profile"] = "production"
			},
			setupEnv: func(t *testing.T) {
				os.Unsetenv("AWS_PROFILE")
			},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &StackParameters{
				Provider:   cliClient.ProviderAWS,
				Region:     "us-west-2",
				Name:       "awsuswest2",
				AWSProfile: "production",
			},
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
				"RequestEnum:aws_profile",
			},
		},
		{
			name: "GCP provider with GCP_PROJECT_ID env var",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Google Cloud Platform"
				m.defaultResponses["region"] = "us-central1"
				m.defaultResponses["stack_name"] = "gcpuscentral1"
				m.defaultResponses["gcp_project_id"] = "my-gcp-project"
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("GCP_PROJECT_ID", "my-gcp-project")
			},
			cleanupEnv: func() {
				os.Unsetenv("GCP_PROJECT_ID")
			},
			expectError: false,
			expectedResult: &StackParameters{
				Provider:     cliClient.ProviderGCP,
				Region:       "us-central1",
				Name:         "gcpuscentral1",
				GCPProjectID: "my-gcp-project",
			},
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
				"RequestStringWithDefault:gcp_project_id",
			},
		},
		{
			name: "GCP provider without GCP_PROJECT_ID env var",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Google Cloud Platform"
				m.defaultResponses["region"] = "europe-west1"
				m.defaultResponses["stack_name"] = "gcpeuropewest1"
				m.responses["gcp_project_id"] = "user-entered-project"
			},
			setupEnv: func(t *testing.T) {
				os.Unsetenv("GCP_PROJECT_ID")
			},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &StackParameters{
				Provider:     cliClient.ProviderGCP,
				Region:       "europe-west1",
				Name:         "gcpeuropewest1",
				GCPProjectID: "user-entered-project",
			},
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
				"RequestString:gcp_project_id",
			},
		},
		{
			name: "Defang Playground provider (no region)",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Defang Playground"
				m.defaultResponses["stack_name"] = "defang"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &StackParameters{
				Provider: cliClient.ProviderDefang,
				Region:   "",
				Name:     "defang",
			},
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:stack_name",
			},
		},
		{
			name: "DigitalOcean provider",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "DigitalOcean"
				m.defaultResponses["region"] = "nyc3"
				m.defaultResponses["stack_name"] = "digitaloceeannyc3"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &StackParameters{
				Provider: cliClient.ProviderDO,
				Region:   "nyc3",
				Name:     "digitaloceeannyc3",
			},
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
			},
		},
		{
			name: "Provider selection error",
			setupMock: func(m *mockElicitationsController) {
				m.enumErrors["provider"] = errors.New("user cancelled")
			},
			setupEnv:          func(t *testing.T) {},
			cleanupEnv:        func() {},
			expectError:       true,
			expectedResult:    nil,
			expectedCallOrder: []string{"RequestEnum:provider"},
		},
		{
			name: "Invalid provider name",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Invalid Provider"
			},
			setupEnv:          func(t *testing.T) {},
			cleanupEnv:        func() {},
			expectError:       true,
			expectedResult:    nil,
			expectedCallOrder: []string{"RequestEnum:provider"},
		},
		{
			name: "Region selection error",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultErrors["region"] = errors.New("region input failed")
			},
			setupEnv:       func(t *testing.T) {},
			cleanupEnv:     func() {},
			expectError:    true,
			expectedResult: nil,
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
			},
		},
		{
			name: "Stack name selection error",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultResponses["region"] = "us-west-2"
				m.defaultErrors["stack_name"] = errors.New("stack name input failed")
			},
			setupEnv:       func(t *testing.T) {},
			cleanupEnv:     func() {},
			expectError:    true,
			expectedResult: nil,
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
			},
		},
		{
			name: "AWS profile selection error",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultResponses["region"] = "us-west-2"
				m.defaultResponses["stack_name"] = "awsuswest2"
				m.enumErrors["aws_profile"] = errors.New("AWS profile input failed")
			},
			setupEnv:       func(t *testing.T) {},
			cleanupEnv:     func() {},
			expectError:    true,
			expectedResult: nil,
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
				"RequestEnum:aws_profile",
			},
		},
		{
			name: "GCP project ID selection error",
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Google Cloud Platform"
				m.defaultResponses["region"] = "us-central1"
				m.defaultResponses["stack_name"] = "gcpuscentral1"
				m.errors["gcp_project_id"] = errors.New("GCP project ID input failed")
			},
			setupEnv: func(t *testing.T) {
				os.Unsetenv("GCP_PROJECT_ID")
			},
			cleanupEnv:     func() {},
			expectError:    true,
			expectedResult: nil,
			expectedCallOrder: []string{
				"RequestEnum:provider",
				"RequestStringWithDefault:region",
				"RequestStringWithDefault:stack_name",
				"RequestString:gcp_project_id",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tt.setupEnv(t)
			defer tt.cleanupEnv()

			mockController := newMockElicitationsController()
			tt.setupMock(mockController)

			wizard := NewWizard(mockController)
			ctx := context.Background()

			// Execute
			result, err := wizard.CollectParameters(ctx)

			// Verify error expectation
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify result
			if tt.expectedResult == nil && result != nil {
				t.Errorf("expected nil result but got %+v", result)
			}
			if tt.expectedResult != nil && result == nil {
				t.Errorf("expected result %+v but got nil", tt.expectedResult)
			}
			if tt.expectedResult != nil && result != nil {
				if result.Provider != tt.expectedResult.Provider {
					t.Errorf("expected Provider %v, got %v", tt.expectedResult.Provider, result.Provider)
				}
				if result.Region != tt.expectedResult.Region {
					t.Errorf("expected Region %v, got %v", tt.expectedResult.Region, result.Region)
				}
				if result.Name != tt.expectedResult.Name {
					t.Errorf("expected Name %v, got %v", tt.expectedResult.Name, result.Name)
				}
				if result.AWSProfile != tt.expectedResult.AWSProfile {
					t.Errorf("expected AWSProfile %v, got %v", tt.expectedResult.AWSProfile, result.AWSProfile)
				}
				if result.GCPProjectID != tt.expectedResult.GCPProjectID {
					t.Errorf("expected GCPProjectID %v, got %v", tt.expectedResult.GCPProjectID, result.GCPProjectID)
				}
			}

			// Verify call order
			if len(mockController.callOrder) != len(tt.expectedCallOrder) {
				t.Errorf("expected %d calls, got %d calls", len(tt.expectedCallOrder), len(mockController.callOrder))
				t.Errorf("expected calls: %v", tt.expectedCallOrder)
				t.Errorf("actual calls: %v", mockController.callOrder)
			} else {
				for i, expectedCall := range tt.expectedCallOrder {
					if i >= len(mockController.callOrder) {
						t.Errorf("expected call %d: %s, but no more calls were made", i, expectedCall)
					} else if mockController.callOrder[i] != expectedCall {
						t.Errorf("expected call %d: %s, got: %s", i, expectedCall, mockController.callOrder[i])
					}
				}
			}
		})
	}
}

func TestWizardSetSupported(t *testing.T) {
	mockController := newMockElicitationsController()
	wizard := NewWizard(mockController)

	// Test initial state
	if !mockController.IsSupported() {
		t.Error("expected controller to be supported by default")
	}

	// Test setting unsupported
	mockController.SetSupported(false)
	if mockController.IsSupported() {
		t.Error("expected controller to be unsupported after SetSupported(false)")
	}

	// Test setting supported again
	mockController.SetSupported(true)
	if !mockController.IsSupported() {
		t.Error("expected controller to be supported after SetSupported(true)")
	}

	_ = wizard // Suppress unused variable warning
}
