package debug

import (
	"context"
	"testing"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockAgent struct {
	mock.Mock
}

func (m *mockAgent) StartWithMessage(ctx context.Context, prompt string) error {
	m.Called(ctx, prompt)
	return nil
}

type mockSurveyor struct {
	response bool
}

func (s *mockSurveyor) AskOne(q survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	b, ok := response.(*bool)
	if !ok {
		panic("response must be a *bool for this mock")
	}
	*b = s.response
	return nil
}

func TestDebugDeployment(t *testing.T) {
	ctx := context.Background()
	mockAgent := &mockAgent{}

	providerID := client.ProviderAWS
	project := compose.Project{}

	tests := []struct {
		name           string
		debugConfig    DebugConfig
		expectedPrompt string
		permission     bool
	}{
		{
			name: "User declines to debug",
			debugConfig: DebugConfig{
				Deployment: "test-deployment",
			},
			expectedPrompt: "",
			permission:     false,
		},
		{
			name: "User agrees to debug",
			debugConfig: DebugConfig{
				Deployment: "test-deployment",
			},
			expectedPrompt: "An error occurred while deploying this project with Defang. Help troubleshoot and recommend a solution. Look at the logs to understand what happened.The deployment ID is \"test-deployment\".",
			permission:     true,
		},
		{
			name: "With rich DebugConfig",
			debugConfig: DebugConfig{
				Deployment:     "test-deployment",
				ProviderID:     &providerID,
				Since:          time.Date(2025, 1, 2, 3, 4, 5, 0, time.Local),
				Until:          time.Date(2025, 1, 2, 4, 5, 6, 0, time.Local),
				FailedServices: []string{"backend"},
				Project:        &project,
			},
			expectedPrompt: "An error occurred while deploying this project to AWS with Defang. Help troubleshoot and recommend a solution. Look at the logs to understand what happened.The deployment ID is \"test-deployment\". The services that failed to deploy are: [backend]. The deployment started at 2025-01-02 03:04:05 -0800 PST. The deployment finished at 2025-01-02 04:05:06 -0800 PST.The compose files are at []. The compose file is as follows:\n\nservices: {}\n",
			permission:     true,
		},
	}

	for _, tt := range tests {
		mockSurveyor := &mockSurveyor{
			response: tt.permission,
		}
		debugger := &Debugger{
			agent:    mockAgent,
			surveyor: mockSurveyor,
		}
		t.Run(tt.name, func(t *testing.T) {
			mockAgent.ExpectedCalls = nil
			mockAgent.Calls = nil

			if tt.permission {
				mockAgent.On("StartWithMessage", ctx, tt.expectedPrompt).Return(nil)
			}

			err := debugger.DebugDeployment(ctx, tt.debugConfig)
			assert.NoError(t, err, "DebugDeployment should not return an error")

			if tt.permission {
				mockAgent.AssertCalled(t, "StartWithMessage", ctx, tt.expectedPrompt)
			} else {
				mockAgent.AssertNotCalled(t, "StartWithMessage", mock.Anything, mock.Anything)
			}
		})
	}
}

func TestDebugComposeLoadError(t *testing.T) {
	ctx := context.Background()
	mockAgent := &mockAgent{}

	tests := []struct {
		name           string
		debugConfig    DebugConfig
		expectedPrompt string
		permission     bool
	}{
		{
			name: "User declines to debug",
			debugConfig: DebugConfig{
				Deployment: "load-error-deployment",
			},
			expectedPrompt: "",
			permission:     false,
		},
		{
			name: "User agrees to debug",
			debugConfig: DebugConfig{
				Deployment: "load-error-deployment",
			},
			expectedPrompt: "The following error occurred while loading the compose file. Help troubleshoot and recommend a solution.validating /Users/jordan/wk/defang/src/testdata/invalid-no-services/compose.yaml:  additional properties 'foo' not allowed",
			permission:     true,
		},
	}

	for _, tt := range tests {
		mockSurveyor := &mockSurveyor{
			response: tt.permission,
		}
		debugger := &Debugger{
			agent:    mockAgent,
			surveyor: mockSurveyor,
		}
		t.Run(tt.name, func(t *testing.T) {
			mockAgent.ExpectedCalls = nil
			mockAgent.Calls = nil

			if tt.permission {
				mockAgent.On("StartWithMessage", ctx, tt.expectedPrompt).Return(nil)
			}

			t.Chdir("../../testdata/invalid-no-services")
			loader := compose.NewLoader()

			_, loadErr := loader.LoadProject(ctx)
			if loadErr != nil {
				term.Error("Cannot load project:", loadErr)
				project, err := loader.CreateProjectForDebug()
				assert.NoError(t, err, "CreateProjectForDebug should not return an error")

				err = debugger.DebugComposeLoadError(ctx, DebugConfig{
					Project: project,
				}, loadErr)
				assert.NoError(t, err, "DebugComposeLoadError should not return an error")
			}

			if tt.permission {
				mockAgent.AssertCalled(t, "StartWithMessage", ctx, tt.expectedPrompt)
			} else {
				mockAgent.AssertNotCalled(t, "StartWithMessage", mock.Anything, mock.Anything)
			}
		})
	}
}
