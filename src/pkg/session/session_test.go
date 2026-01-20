package session

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockElicitationsController struct {
	isSupported bool
	enumChoice  string
}

func (m *mockElicitationsController) RequestString(ctx context.Context, message, field string) (string, error) {
	return "", nil
}

func (m *mockElicitationsController) RequestStringWithDefault(ctx context.Context, message, field, defaultValue string) (string, error) {
	return defaultValue, nil
}

func (m *mockElicitationsController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	if m.enumChoice != "" {
		return m.enumChoice, nil
	}
	if len(options) > 0 {
		return options[0], nil
	}
	return "", nil
}

func (m *mockElicitationsController) SetSupported(supported bool) {
	m.isSupported = supported
}

func (m *mockElicitationsController) IsSupported() bool {
	return m.isSupported
}

type mockStacksManager struct {
	mock.Mock
}

func (m *mockStacksManager) List(ctx context.Context) ([]stacks.ListItem, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).([]stacks.ListItem)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *mockStacksManager) LoadLocal(name string) (*stacks.Parameters, error) {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).(*stacks.Parameters)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *mockStacksManager) GetRemote(ctx context.Context, name string) (*stacks.Parameters, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	result, ok := args.Get(0).(*stacks.Parameters)
	if !ok {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *mockStacksManager) Create(params stacks.Parameters) (string, error) {
	args := m.Called(params)
	return args.String(0), args.Error(1)
}

func (m *mockStacksManager) GetStack(ctx context.Context, opts stacks.GetStackOpts) (*stacks.Parameters, string, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, "", args.Error(2)
	}
	result, ok := args.Get(0).(*stacks.Parameters)
	if !ok {
		return nil, "", args.Error(2)
	}
	whence, _ := args.Get(1).(string)
	return result, whence, args.Error(2)
}

func (m *mockStacksManager) TargetDirectory() string {
	return ""
}

func TestLoadSession(t *testing.T) {
	deployedAt := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		options       SessionLoaderOptions
		localStack    *stacks.Parameters
		remoteStack   *stacks.Parameters
		stacksList    []stacks.ListItem
		expectedError string
		expectedStack *stacks.Parameters
		expectedEnv   map[string]string
	}{
		{
			name:    "empty options - fallback stack",
			options: SessionLoaderOptions{},
			expectedStack: &stacks.Parameters{
				Name: "beta",
			},
		},
		{
			name: "only project name specified",
			options: SessionLoaderOptions{
				ProjectName: "foo",
			},
			expectedStack: &stacks.Parameters{
				Name: "beta",
			},
		},
		{
			name: "provider specified without stack assumes beta stack",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				ProviderID:  client.ProviderAWS,
			},
			expectedStack: &stacks.Parameters{
				Name:      "beta",
				Provider:  client.ProviderAWS,
				Variables: map[string]string{},
			},
			expectedError: "",
		},
		{
			name: "stack specified but not found",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				GetStackOpts: stacks.GetStackOpts{
					Stack: "missing-stack",
				},
			},
			expectedError: "unable to find stack",
			expectedEnv:   map[string]string{},
		},
		{
			name: "local stack specified",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				GetStackOpts: stacks.GetStackOpts{
					Stack: "local-stack",
				},
			},
			localStack: &stacks.Parameters{
				Name:     "local-stack",
				Provider: client.ProviderDefang,
				Region:   "us-test-2",
				Variables: map[string]string{
					"AWS_PROFILE": "default",
					"FOO":         "bar",
				},
			},
			expectedStack: &stacks.Parameters{
				Name:      "local-stack",
				Provider:  client.ProviderDefang,
				Variables: map[string]string{},
			},
			expectedEnv: map[string]string{
				"AWS_PROFILE": "default",
				"FOO":         "bar",
			},
		},
		{
			name: "remote stack specified",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				GetStackOpts: stacks.GetStackOpts{
					Stack: "remote-stack",
				},
			},
			remoteStack: &stacks.Parameters{
				Name:     "remote-stack",
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-gcp-project",
					"FOO":            "bar",
				},
			},
			expectedStack: &stacks.Parameters{
				Name:      "remote-stack",
				Provider:  client.ProviderGCP,
				Variables: map[string]string{},
			},
			expectedEnv: map[string]string{
				"GCP_PROJECT_ID": "my-gcp-project",
				"FOO":            "bar",
			},
		},
		{
			name: "local and remote stack",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				GetStackOpts: stacks.GetStackOpts{
					Stack: "both-stack",
				},
			},
			localStack: &stacks.Parameters{
				Name:     "both-stack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"AWS_PROFILE": "local-profile",
					"FOO":         "local-bar",
				},
			},
			remoteStack: &stacks.Parameters{
				Name:     "both-stack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"AWS_PROFILE": "remote-profile",
					"FOO":         "remote-bar",
				},
			},
			expectedStack: &stacks.Parameters{
				Name:     "both-stack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"AWS_PROFILE": "local-profile",
					"FOO":         "local-bar",
				},
			},
			expectedEnv: map[string]string{
				"AWS_PROFILE": "local-profile",
				"FOO":         "local-bar",
			},
		},
		{
			name: "interactive selection - stack required",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				ProviderID:  client.ProviderGCP,
				GetStackOpts: stacks.GetStackOpts{
					Interactive:        true,
					AllowStackCreation: true,
					RequireStack:       true,
				},
			},
			stacksList: []stacks.ListItem{
				{
					Parameters: stacks.Parameters{
						Name:     "existing-stack",
						Provider: client.ProviderGCP,
						Region:   "us-central1",
						Variables: map[string]string{
							"GCP_PROJECT": "existing-gcp-project",
							"FOO":         "existing-bar",
						},
					},
					DeployedAt: deployedAt,
				},
			},
			expectedStack: &stacks.Parameters{
				Name:      "existing-stack",
				Provider:  client.ProviderGCP,
				Variables: map[string]string{},
			},
			expectedEnv: map[string]string{
				"GCP_PROJECT": "existing-gcp-project",
				"FOO":         "existing-bar",
			},
		},
		{
			name: "interactive selection",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				ProviderID:  client.ProviderGCP,
				GetStackOpts: stacks.GetStackOpts{
					Interactive:        true,
					AllowStackCreation: true,
				},
			},
			stacksList: []stacks.ListItem{
				{
					Parameters: stacks.Parameters{
						Name:     "existing-stack",
						Provider: client.ProviderAWS,
						Region:   "us-test-2",
						Variables: map[string]string{
							"FOO": "existing-bar",
						},
					},
					DeployedAt: deployedAt,
				},
			},
			expectedStack: &stacks.Parameters{
				Name:     "beta",
				Provider: client.ProviderGCP,
			},
		},
		{
			name: "stack with compose vars updates loader",
			options: SessionLoaderOptions{
				ProjectName: "foo",
				GetStackOpts: stacks.GetStackOpts{
					Stack: "compose-stack",
				},
			},
			localStack: &stacks.Parameters{
				Name:     "compose-stack",
				Provider: client.ProviderDefang,
				Region:   "us-test-2",
				Variables: map[string]string{
					"COMPOSE_PROJECT_NAME": "myproject",
					"COMPOSE_PATH":         "./docker-compose.yml:./docker-compose.override.yml",
				},
			},
			expectedStack: &stacks.Parameters{
				Name:     "compose-stack",
				Provider: client.ProviderDefang,
				Variables: map[string]string{
					"COMPOSE_PROJECT_NAME": "myproject",
					"COMPOSE_PATH":         "./docker-compose.yml:./docker-compose.override.yml",
				},
			},
			expectedEnv: map[string]string{
				"COMPOSE_PROJECT_NAME": "myproject",
				"COMPOSE_PATH":         "./docker-compose.yml:./docker-compose.override.yml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key := range tt.expectedEnv {
				os.Unsetenv(key)
			}
			t.Cleanup(func() {
				for key := range tt.expectedEnv {
					os.Unsetenv(key)
				}
			})
			ctx := t.Context()
			sm := &mockStacksManager{}

			// Setup mock expectations based on test case
			if tt.localStack != nil {
				sm.On("LoadLocal", tt.localStack.Name).Return(tt.localStack, nil)
			} else {
				sm.On("LoadLocal", mock.Anything).Maybe().Return(nil, os.ErrNotExist)
			}

			if tt.remoteStack != nil {
				sm.On("GetRemote", ctx, tt.remoteStack.Name).Maybe().Return(tt.remoteStack, nil)
				sm.On("Create", *tt.remoteStack).Maybe().Return("", nil)
			} else {
				sm.On("GetRemote", ctx, mock.Anything).Maybe().Return(nil, errors.New("unable to find stack"))
			}
			if tt.stacksList != nil {
				sm.On("List", ctx).Maybe().Return(tt.stacksList, nil)
			}

			loader := NewSessionLoader(client.MockFabricClient{}, sm, tt.options)
			session, err := loader.LoadSession(ctx)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, session)

			// Verify session contents
			assert.NotNil(t, session.Loader)

			assert.NotNil(t, session.Provider)
			if tt.options.ProviderID == client.ProviderAWS {
				_, ok := session.Provider.(*aws.ByocAws)
				assert.True(t, ok)
			}
			if tt.options.ProviderID == client.ProviderGCP {
				_, ok := session.Provider.(*gcp.ByocGcp)
				assert.True(t, ok)
			}

			assert.NotNil(t, session.Stack)
			assert.Equal(t, tt.expectedStack.Name, session.Stack.Name)
			assert.Equal(t, tt.expectedStack.Provider, session.Stack.Provider)

			// Verify environment variables
			for key, expectedValue := range tt.expectedEnv {
				actualValue, exists := session.Stack.Variables[key]
				assert.True(t, exists, "expected env var %s to be set", key)
				assert.Equal(t, expectedValue, actualValue, "env var %s has unexpected value", key)
				assert.Equal(t, expectedValue, os.Getenv(key))
			}

			// Verify all mock expectations were met
			sm.AssertExpectations(t)
		})
	}
}

func TestLoadSession_NoStackManager(t *testing.T) {
	ctx := t.Context()

	options := SessionLoaderOptions{
		ProviderID: client.ProviderDefang,
	}

	loader := NewSessionLoader(client.MockFabricClient{}, nil, options)
	session, err := loader.LoadSession(ctx)
	require.NoError(t, err)
	assert.NotNil(t, session)
}
