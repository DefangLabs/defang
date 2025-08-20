package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSurveyor implements the Surveyor interface for testing
type MockSurveyor struct {
	mock.Mock
	responses []interface{}
	callIndex int
}

func (m *MockSurveyor) AskOne(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	args := m.Called(prompt, response, opts)
	if m.callIndex < len(m.responses) {
		// Set the response value based on the type
		switch r := response.(type) {
		case *string:
			if val, ok := m.responses[m.callIndex].(string); ok {
				*r = val
			}
		}
		m.callIndex++
	}
	return args.Error(0)
}

// MockFabricClient implements the FabricClient interface for testing
type MockFabricClient struct {
	mock.Mock
	generateComposeRequests []*defangv1.GenerateComposeRequest
	client.FabricClient
}

func (m *MockFabricClient) GenerateCompose(ctx context.Context, req *defangv1.GenerateComposeRequest) (*defangv1.GenerateComposeResponse, error) {
	m.generateComposeRequests = append(m.generateComposeRequests, req)
	args := m.Called(ctx, req)
	resp, ok := args.Get(0).(*defangv1.GenerateComposeResponse)
	if !ok && args.Get(0) != nil {
		return nil, errors.New("failed to cast to *defangv1.GenerateComposeResponse")
	}
	return resp, args.Error(1)
}

// MockHerokuClient for testing Heroku operations
type MockHerokuClient struct {
	mock.Mock
}

func (m *MockHerokuClient) ListApps(ctx context.Context) ([]HerokuApplication, error) {
	args := m.Called(ctx)
	apps, ok := args.Get(0).([]HerokuApplication)
	if !ok {
		return nil, errors.New("failed to cast to []HerokuApplication")
	}
	return apps, args.Error(1)
}

func (m *MockHerokuClient) ListDynos(ctx context.Context, appName string) ([]HerokuDyno, error) {
	args := m.Called(ctx, appName)
	dynos, ok := args.Get(0).([]HerokuDyno)
	if !ok {
		return nil, errors.New("failed to cast to []HerokuDyno")
	}
	return dynos, args.Error(1)
}

func (m *MockHerokuClient) ListAddons(ctx context.Context, appName string) ([]HerokuAddon, error) {
	args := m.Called(ctx, appName)
	addons, ok := args.Get(0).([]HerokuAddon)
	if !ok {
		return nil, errors.New("failed to cast to []HerokuAddon")
	}
	return addons, args.Error(1)
}

func (m *MockHerokuClient) GetPGInfo(ctx context.Context, addonID string) (PGInfo, error) {
	args := m.Called(ctx, addonID)
	pgInfo, ok := args.Get(0).(PGInfo)
	if !ok {
		return PGInfo{}, errors.New("failed to cast to *PGInfo")
	}
	return pgInfo, args.Error(1)
}

func (m *MockHerokuClient) ListConfigVars(ctx context.Context, appName string) (HerokuConfigVars, error) {
	args := m.Called(ctx, appName)
	configVars, ok := args.Get(0).(HerokuConfigVars)
	if !ok {
		return nil, errors.New("failed to cast to HerokuConfigVars")
	}
	return configVars, args.Error(1)
}

func (m *MockHerokuClient) SetToken(token string) {
	m.Called(token)
}

func TestInteractiveSetup(t *testing.T) {
	tests := []struct {
		name                        string
		sourcePlatform              SourcePlatform
		surveyResponses             []interface{}
		herokuToken                 string
		herokuApps                  []HerokuApplication
		herokuDynos                 []HerokuDyno
		herokuAddons                []HerokuAddon
		herokuPGInfo                []PGInfo
		herokuConfigVars            HerokuConfigVars
		composeResponse             *defangv1.GenerateComposeResponse
		expectedComposeFileContents string
		composeError                error
		expectError                 bool
		expectedCalls               int
	}{
		{
			name:           "successful setup with heroku platform preselected",
			sourcePlatform: SourcePlatformHeroku,
			surveyResponses: []interface{}{
				"my-test-app", // selectSourceApplication response
			},
			herokuToken: "test-token",
			herokuApps: []HerokuApplication{
				{Name: "my-test-app", ID: "app-123"},
				{Name: "another-app", ID: "app-456"},
			},
			herokuDynos: []HerokuDyno{
				{Name: "web.1", Command: "npm start", Size: "Standard-1X", Type: "web"},
			},
			herokuAddons: []HerokuAddon{
				{
					Name: "postgresql-addon-123",
					ID:   "addon-123",
					AddonService: struct {
						HumanName string `json:"human_name"`
						ID        string `json:"id"`
						Name      string `json:"name"`
					}{HumanName: "Heroku Postgres", ID: "service-123", Name: "heroku-postgresql"},
					Plan: struct {
						HumanName string `json:"human_name"`
						ID        string `json:"id"`
						Name      string `json:"name"`
					}{HumanName: "Mini", ID: "plan-123", Name: "heroku-postgresql:mini"},
					State: "provisioned",
				},
			},
			herokuPGInfo: []PGInfo{
				{
					DatabaseName: "mydb",
					NumBytes:     12345,
					Info: []struct {
						Name   string   `json:"name"`
						Values []string `json:"values"`
					}{
						{Name: "PG Version", Values: []string{"17.4"}},
					},
				},
			},
			herokuConfigVars: HerokuConfigVars{
				"NODE_ENV": "production",
				"PORT":     "3000",
			},
			composeResponse: &defangv1.GenerateComposeResponse{
				Compose: []byte("version: '3.8'\nservices:\n  web:\n    image: my-app:latest"),
			},
			expectError:                 false,
			expectedCalls:               1, // Successful compose generation should only call once
			expectedComposeFileContents: "services:\n    web:\n        image: my-app:latest\n",
		},
		{
			name:           "successful setup with platform selection",
			sourcePlatform: "",
			surveyResponses: []interface{}{
				"heroku",      // selectSourcePlatform response
				"my-test-app", // selectSourceApplication response
			},
			herokuToken: "test-token",
			herokuApps: []HerokuApplication{
				{Name: "my-test-app", ID: "app-123"},
			},
			herokuDynos: []HerokuDyno{
				{Name: "web.1", Command: "node server.js", Size: "Standard-2X", Type: "web"},
				{Name: "web.2", Command: "node server.js", Size: "Standard-2X", Type: "web"},
			},
			herokuAddons: []HerokuAddon{
				{
					Name: "redis-addon-456",
					ID:   "addon-456",
					AddonService: struct {
						HumanName string `json:"human_name"`
						ID        string `json:"id"`
						Name      string `json:"name"`
					}{HumanName: "Heroku Redis", ID: "service-456", Name: "heroku-redis"},
					Plan: struct {
						HumanName string `json:"human_name"`
						ID        string `json:"id"`
						Name      string `json:"name"`
					}{HumanName: "Mini", ID: "plan-456", Name: "heroku-redis:mini"},
					State: "provisioned",
				},
			},
			herokuConfigVars: HerokuConfigVars{
				"REDIS_URL": "redis://localhost:6379",
			},
			composeResponse: &defangv1.GenerateComposeResponse{
				Compose: []byte("version: '3.8'\nservices:\n  web:\n    image: redis-app:latest"),
			},
			expectedComposeFileContents: "services:\n    web:\n        image: redis-app:latest\n",
			expectError:                 false,
			expectedCalls:               1, // Successful compose generation should only call once
		},
		{
			name:           "fabric client error during compose generation",
			sourcePlatform: SourcePlatformHeroku,
			surveyResponses: []interface{}{
				"failing-app",
			},
			herokuToken: "test-token",
			herokuApps: []HerokuApplication{
				{Name: "failing-app", ID: "app-789"},
			},
			herokuDynos:      []HerokuDyno{{Name: "web.1", Command: "python app.py", Type: "web", Size: "Standard-1X"}},
			herokuAddons:     []HerokuAddon{},
			herokuConfigVars: HerokuConfigVars{},
			composeError:     errors.New("fabric service unavailable"),
			expectError:      true,
			expectedCalls:    1, // Fabric client errors are not retried, only called once
		},
		{
			name:           "invalid yaml response from fabric client",
			sourcePlatform: SourcePlatformHeroku,
			surveyResponses: []interface{}{
				"yaml-invalid-app",
			},
			herokuToken: "test-token",
			herokuApps: []HerokuApplication{
				{Name: "yaml-invalid-app", ID: "app-invalid"},
			},
			herokuDynos:      []HerokuDyno{{Name: "web.1", Command: "python app.py", Type: "web", Size: "Standard-1X"}},
			herokuAddons:     []HerokuAddon{},
			herokuConfigVars: HerokuConfigVars{},
			composeResponse: &defangv1.GenerateComposeResponse{
				Compose: []byte("invalid: yaml: content: [unclosed"),
			},
			expectError:   true,
			expectedCalls: 3, // Retries 3 times due to YAML unmarshal error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			if tt.herokuToken != "" {
				t.Setenv("HEROKU_API_KEY", tt.herokuToken)
				defer os.Unsetenv("HEROKU_API_KEY")
			}

			// Create mock surveyor
			mockSurveyor := &MockSurveyor{
				responses: tt.surveyResponses,
			}

			// Set up surveyor expectations
			for range tt.surveyResponses {
				mockSurveyor.On("AskOne", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}

			// Create mock fabric client
			mockFabricClient := &MockFabricClient{}

			if tt.composeError != nil {
				// Fabric client errors are returned immediately, no retry
				mockFabricClient.On("GenerateCompose", mock.Anything, mock.Anything).Return(
					(*defangv1.GenerateComposeResponse)(nil), tt.composeError).Once()
			} else if tt.composeResponse != nil {
				if tt.name == "invalid yaml response from fabric client" {
					// For invalid YAML, it will retry 3 times because YAML unmarshal fails
					mockFabricClient.On("GenerateCompose", mock.Anything, mock.Anything).Return(
						tt.composeResponse, nil).Times(3)
				} else {
					// Successful cases call only once since valid YAML is returned
					mockFabricClient.On("GenerateCompose", mock.Anything, mock.Anything).Return(
						tt.composeResponse, nil).Once()
				}
			}

			// Create mock Heroku client
			mockHerokuClient := &MockHerokuClient{}

			// Set up Heroku client expectations
			mockHerokuClient.On("SetToken", tt.herokuToken).Once()
			mockHerokuClient.On("ListApps", mock.Anything).Return(tt.herokuApps, nil)
			mockHerokuClient.On("ListDynos", mock.Anything, mock.Anything).Return(tt.herokuDynos, nil)
			mockHerokuClient.On("GetPGInfo", mock.Anything, mock.Anything).Return(tt.herokuPGInfo, nil)
			mockHerokuClient.On("ListAddons", mock.Anything, mock.Anything).Return(tt.herokuAddons, nil)
			mockHerokuClient.On("ListConfigVars", mock.Anything, mock.Anything).Return(tt.herokuConfigVars, nil)

			// Execute the function under test
			ctx := context.Background()
			composeFileContents, err := InteractiveSetup(ctx, mockFabricClient, mockSurveyor, mockHerokuClient, tt.sourcePlatform)

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockSurveyor.AssertExpectations(t)
			mockFabricClient.AssertExpectations(t)
			mockHerokuClient.AssertExpectations(t)

			// Verify fabric client was called the expected number of times
			assert.Len(t, mockFabricClient.generateComposeRequests, tt.expectedCalls)

			// Verify fabric client request payload if calls were made
			if len(mockFabricClient.generateComposeRequests) > 0 {
				req := mockFabricClient.generateComposeRequests[0]
				assert.Equal(t, defangv1.SourcePlatform_SOURCE_PLATFORM_HEROKU, req.Platform)

				// Verify the data payload contains expected Heroku application info
				var appInfo HerokuApplicationInfo
				err := json.Unmarshal(req.Data, &appInfo)
				assert.NoError(t, err)
				assert.Equal(t, tt.herokuDynos, appInfo.Dynos)
				assert.Equal(t, tt.herokuAddons, appInfo.Addons)
				assert.Equal(t, tt.herokuConfigVars, appInfo.ConfigVars)
			}

			assert.Equal(t, tt.expectedComposeFileContents, composeFileContents)
		})
	}
}

func TestCleanupComposeFile(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expected       string
		expectingError bool
	}{
		{
			name: "compose with version",
			input: `version: '3.8'
services:
  web:
    image: my-app:latest`,
			expected: `services:
    web:
        image: my-app:latest
`,
			expectingError: false,
		},
		{
			name: "compose without version",
			input: `services:
  web:
    image: my-app:latest`,
			expected: `services:
    web:
        image: my-app:latest
`,
			expectingError: false,
		},
		{
			name: "invalid yaml",
			input: `version: '3.8'
services:
  web
    image: my-app:latest`, // Missing colon after 'web'
			expected:       "",
			expectingError: true,
		},
		{
			name: "postgres",
			input: `
services:
  db:
    image: postgres:latest`,
			expected: `services:
    db:
        image: postgres:latest
        x-defang-postgres: "true"
`,
			expectingError: false,
		},
		{
			name: "redis",
			input: `
services:
  db:
    image: redis:latest`,
			expected: `services:
    db:
        image: redis:latest
        x-defang-redis: "true"
`,
			expectingError: false,
		},
		{
			name: "with comments",
			input: `
services:
  # newline comment
  web:
    image: my-app:latest
  db:
    image: postgres:latest # EOL comment
`,
			expected: `services:
    # newline comment
    web:
        image: my-app:latest
    db:
        image: postgres:latest # EOL comment
        x-defang-postgres: "true"
`,
			expectingError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := cleanupComposeFile(tt.input)
			if tt.expectingError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExtractFirstCodeBlock(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "single code block",
			input:    "```\nversion: '3.8'\nservices:\n  web:\n    image: my-app:latest\n```",
			expected: "version: '3.8'\nservices:\n  web:\n    image: my-app:latest",
		},
		{
			name:     "single code block with language tag",
			input:    "```yaml\nversion: '3.8'\nservices:\n  web:\n    image: my-app:latest\n```",
			expected: "version: '3.8'\nservices:\n  web:\n    image: my-app:latest",
		},
		{
			name:     "multiple code blocks",
			input:    "Some text\n```yaml\nversion: '3.8'\nservices:\n  web:\n    image: my-app:latest\n```\nMore text\n```json\n{\"key\": \"value\"}\n```",
			expected: "version: '3.8'\nservices:\n  web:\n    image: my-app:latest",
		},
		{
			name:     "code block with surrounding text",
			input:    "Here is some text before the code block.\n```yaml\nversion: '3.8'\nservices:\n  web:\n    image: my-app:latest\n```\nAnd some text after.",
			expected: "version: '3.8'\nservices:\n  web:\n    image: my-app:latest",
		},
		{
			name:        "empty code block",
			input:       "Some text before.\n```\n```\nSome text after.",
			expected:    "",
			expectError: true,
		},
		{
			name:     "code block with extra backticks",
			input:    "Text before.\n````yaml\nversion: '3.8'\nservices:\n  web:\n    image: my-app:latest\n````\nText after.",
			expected: "version: '3.8'\nservices:\n  web:\n    image: my-app:latest",
		},
		{
			name:        "partial code block",
			input:       "Some text\n```\nversion: '3.8'\nservices:\n  web:\n    image: my-app:latest",
			expected:    "",
			expectError: true, // No closing backticks
		},
		{
			name:        "no code blocks",
			input:       "Just some text without code blocks.",
			expectError: true, // No code blocks found
		},
		{
			name:        "empty input",
			input:       "",
			expectError: true, // No code blocks in empty input
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractFirstCodeBlock(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}
