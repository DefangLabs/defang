package stacks

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/stretchr/testify/assert"
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

// mockAWSProfileLister is a mock implementation of AWSProfileLister
type mockAWSProfileLister struct {
	profiles []string
	err      error
}

func (m *mockAWSProfileLister) ListProfiles() ([]string, error) {
	return m.profiles, m.err
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
		mockProfileLister *mockAWSProfileLister
		setupEnv          func(*testing.T)
		cleanupEnv        func()
		expectError       bool
		expectedResult    *Parameters
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
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-east-1",
				Name:     "awsuseast1",
				Variables: map[string]string{
					"AWS_PROFILE": "test-profile",
				},
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
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Name:     "awsuswest2",
				Variables: map[string]string{
					"AWS_PROFILE": "production",
				},
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
			expectedResult: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Name:     "gcpuscentral1",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-gcp-project",
				},
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
			expectedResult: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "europe-west1",
				Name:     "gcpeuropewest1",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "user-entered-project",
				},
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
			expectedResult: &Parameters{
				Provider: client.ProviderDefang,
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
			expectedResult: &Parameters{
				Provider: client.ProviderDO,
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
			mockProfileLister: &mockAWSProfileLister{
				profiles: []string{"default", "staging", "prod"},
				err:      nil,
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
			t.Cleanup(tt.cleanupEnv)

			mockController := newMockElicitationsController()
			tt.setupMock(mockController)

			var profileLister AWSProfileLister
			if tt.mockProfileLister != nil {
				profileLister = tt.mockProfileLister
			} else {
				profileLister = &mockAWSProfileLister{
					profiles: []string{"default", "staging", "prod"},
					err:      nil,
				}
			}

			wizard := NewWizardWithProfileLister(mockController, profileLister)
			ctx := t.Context()

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
			if tt.expectedResult == nil {
				assert.Nil(t, result, "expected nil result")
			} else {
				assert.NotNil(t, result, "expected non-nil result")
				assert.Equal(t, tt.expectedResult.Provider, result.Provider, "Provider mismatch")
				assert.Equal(t, tt.expectedResult.Region, result.Region, "Region mismatch")
				assert.Equal(t, tt.expectedResult.Name, result.Name, "Name mismatch")
				assert.Equal(t, tt.expectedResult.Variables["AWS_PROFILE"], result.Variables["AWS_PROFILE"], "AWS_PROFILE mismatch")
				assert.Equal(t, tt.expectedResult.Variables["GCP_PROJECT_ID"], result.Variables["GCP_PROJECT_ID"], "GCP_PROJECT_ID mismatch")
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

func TestWizardCollectRemainingParameters(t *testing.T) {
	tests := []struct {
		name           string
		initialParams  *Parameters
		setupMock      func(*mockElicitationsController)
		setupEnv       func(*testing.T)
		cleanupEnv     func()
		expectError    bool
		expectedResult *Parameters
	}{
		{
			name: "Only provider missing - AWS",
			initialParams: &Parameters{
				Region: "us-east-1",
				Name:   "my-stack",
			},
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultResponses["aws_profile"] = "default"
				m.enumResponses["aws_profile"] = "default"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-east-1",
				Name:     "my-stack",
				Variables: map[string]string{
					"AWS_PROFILE": "default",
				},
			},
		},
		{
			name: "Only region missing - AWS",
			initialParams: &Parameters{
				Provider: client.ProviderAWS,
				Name:     "my-stack",
			},
			setupMock: func(m *mockElicitationsController) {
				m.defaultResponses["region"] = "us-west-2"
				m.defaultResponses["aws_profile"] = "default"
				m.enumResponses["aws_profile"] = "default"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Name:     "my-stack",
				Variables: map[string]string{
					"AWS_PROFILE": "default",
				},
			},
		},
		{
			name: "Only stack name missing - GCP",
			initialParams: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
			},
			setupMock: func(m *mockElicitationsController) {
				m.defaultResponses["stack_name"] = "gcpuscentral1"
				m.responses["gcp_project_id"] = "my-project"
				m.defaultResponses["gcp_project_id"] = "my-project"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Name:     "gcpuscentral1",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-project",
				},
			},
		},
		{
			name: "Only AWS profile missing - with AWS_PROFILE env",
			initialParams: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "eu-west-1",
				Name:     "my-aws-stack",
			},
			setupMock: func(m *mockElicitationsController) {
				m.defaultResponses["aws_profile"] = "production"
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("AWS_PROFILE", "production")
			},
			cleanupEnv: func() {
				os.Unsetenv("AWS_PROFILE")
			},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "eu-west-1",
				Name:     "my-aws-stack",
				Variables: map[string]string{
					"AWS_PROFILE": "production",
				},
			},
		},
		{
			name: "Only GCP project ID missing - with GCP_PROJECT_ID env",
			initialParams: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "europe-west1",
				Name:     "my-gcp-stack",
			},
			setupMock: func(m *mockElicitationsController) {
				m.defaultResponses["gcp_project_id"] = "env-project-123"
			},
			setupEnv: func(t *testing.T) {
				t.Setenv("GCP_PROJECT_ID", "env-project-123")
			},
			cleanupEnv: func() {
				os.Unsetenv("GCP_PROJECT_ID")
			},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "europe-west1",
				Name:     "my-gcp-stack",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "env-project-123",
				},
			},
		},
		{
			name: "Provider and region missing - DigitalOcean",
			initialParams: &Parameters{
				Name: "do-stack",
			},
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "DigitalOcean"
				m.defaultResponses["region"] = "sfo3"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderDO,
				Region:   "sfo3",
				Name:     "do-stack",
			},
		},
		{
			name: "Provider and name missing - Defang (no region needed)",
			initialParams: &Parameters{
				Region: "should-be-ignored",
			},
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Defang Playground"
				m.defaultResponses["stack_name"] = "defang-playground"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderDefang,
				Region:   "",
				Name:     "defang-playground",
			},
		},
		{
			name: "Region and name missing - AWS",
			initialParams: &Parameters{
				Provider: client.ProviderAWS,
			},
			setupMock: func(m *mockElicitationsController) {
				m.defaultResponses["region"] = "ap-southeast-1"
				m.defaultResponses["stack_name"] = "awsapsoutheast1"
				m.enumResponses["aws_profile"] = "staging"
				m.defaultResponses["aws_profile"] = "staging"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "ap-southeast-1",
				Name:     "awsapsoutheast1",
				Variables: map[string]string{
					"AWS_PROFILE": "staging",
				},
			},
		},
		{
			name: "All parameters provided - AWS complete",
			initialParams: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-1",
				Name:     "complete-stack",
				Variables: map[string]string{
					"AWS_PROFILE": "prod",
				},
			},
			setupMock:   func(m *mockElicitationsController) {},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-1",
				Name:     "complete-stack",
				Variables: map[string]string{
					"AWS_PROFILE": "prod",
				},
			},
		},
		{
			name: "All parameters provided - GCP complete",
			initialParams: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "asia-east1",
				Name:     "gcp-complete",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-complete-project",
				},
			},
			setupMock:   func(m *mockElicitationsController) {},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "asia-east1",
				Name:     "gcp-complete",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-complete-project",
				},
			},
		},
		{
			name: "Defang provider with name - no region needed",
			initialParams: &Parameters{
				Provider: client.ProviderDefang,
				Name:     "my-defang-stack",
			},
			setupMock:   func(m *mockElicitationsController) {},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderDefang,
				Region:   "",
				Name:     "my-defang-stack",
			},
		},
		{
			name:          "Everything missing - AWS from scratch",
			initialParams: &Parameters{},
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "AWS"
				m.defaultResponses["region"] = "us-east-1"
				m.defaultResponses["stack_name"] = "awsuseast1"
				m.enumResponses["aws_profile"] = "default"
				m.defaultResponses["aws_profile"] = "default"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-east-1",
				Name:     "awsuseast1",
				Variables: map[string]string{
					"AWS_PROFILE": "default",
				},
			},
		},
		{
			name:          "Everything missing - GCP from scratch",
			initialParams: &Parameters{},
			setupMock: func(m *mockElicitationsController) {
				m.enumResponses["provider"] = "Google Cloud Platform"
				m.defaultResponses["region"] = "us-central1"
				m.defaultResponses["stack_name"] = "gcpuscentral1"
				m.responses["gcp_project_id"] = "my-gcp-project"
				m.defaultResponses["gcp_project_id"] = "my-gcp-project"
			},
			setupEnv:    func(t *testing.T) {},
			cleanupEnv:  func() {},
			expectError: false,
			expectedResult: &Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Name:     "gcpuscentral1",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-gcp-project",
				},
			},
		},
		{
			name: "Provider missing with error during selection",
			initialParams: &Parameters{
				Region: "us-east-1",
				Name:   "error-stack",
			},
			setupMock: func(m *mockElicitationsController) {
				m.enumErrors["provider"] = errors.New("provider selection cancelled")
			},
			setupEnv:       func(t *testing.T) {},
			cleanupEnv:     func() {},
			expectError:    true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tt.setupEnv(t)
			t.Cleanup(tt.cleanupEnv)

			mockController := newMockElicitationsController()
			tt.setupMock(mockController)

			wizard := NewWizard(mockController)
			ctx := t.Context()

			// Execute
			result, err := wizard.CollectRemainingParameters(ctx, tt.initialParams)

			// Verify error expectation
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify result using assert
			if tt.expectedResult == nil {
				assert.Nil(t, result, "expected nil result")
			} else {
				assert.NotNil(t, result, "expected non-nil result")
				assert.Equal(t, tt.expectedResult.Provider, result.Provider, "Provider mismatch")
				assert.Equal(t, tt.expectedResult.Region, result.Region, "Region mismatch")
				assert.Equal(t, tt.expectedResult.Name, result.Name, "Name mismatch")
				assert.Equal(t, tt.expectedResult.Variables["AWS_PROFILE"], result.Variables["AWS_PROFILE"], "AWS_PROFILE mismatch")
				assert.Equal(t, tt.expectedResult.Variables["GCP_PROJECT_ID"], result.Variables["GCP_PROJECT_ID"], "GCP_PROJECT_ID mismatch")
			}
		})
	}
}
