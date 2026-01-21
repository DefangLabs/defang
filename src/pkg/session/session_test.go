package session

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

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
	tests := []struct {
		name          string
		options       SessionLoaderOptions
		existingStack *stacks.Parameters
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
			name: "specified non-existing stack",
			options: SessionLoaderOptions{
				GetStackOpts: stacks.GetStackOpts{
					Stack: "missingstack",
				},
			},

			expectedError: "stack \"missingstack\" does not exist",
			expectedEnv:   map[string]string{},
		},
		{
			name: "specified existing stack",
			options: SessionLoaderOptions{
				GetStackOpts: stacks.GetStackOpts{
					Stack: "existingstack",
				},
			},
			existingStack: &stacks.Parameters{
				Name:     "existingstack",
				Provider: client.ProviderDefang,
			},
			expectedStack: &stacks.Parameters{
				Name:     "existingstack",
				Provider: client.ProviderDefang,
				Variables: map[string]string{
					"DEFANG_PROVIDER": "defang",
				},
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

			if tt.existingStack == nil {
				if tt.options.GetStackOpts.Stack != "" {
					// For specified non-existing stack, return ErrNotExist
					sm.On("GetStack", ctx, mock.Anything).Maybe().Return(nil, "", &stacks.ErrNotExist{StackName: tt.options.GetStackOpts.Stack})
				} else {
					// For empty stack (should fall back to beta), return a general error that's not ErrNotExist
					sm.On("GetStack", ctx, mock.Anything).Maybe().Return(nil, "", errors.New("no default stack set for project"))
				}
			} else {
				sm.On("GetStack", ctx, mock.Anything).Maybe().Return(tt.existingStack, "local", nil)
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
