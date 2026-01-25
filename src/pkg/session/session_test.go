package session

import (
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/modes"
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
		getStackError error
		expectedError string
		expectedStack *stacks.Parameters
		expectedEnv   map[string]string
	}{
		{
			name: "specified stack",
			options: SessionLoaderOptions{
				GetStackOpts: stacks.GetStackOpts{
					Default: stacks.Parameters{
						Name: "existingstack",
					},
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
			name: "override mode",
			options: SessionLoaderOptions{
				GetStackOpts: stacks.GetStackOpts{
					Default: stacks.Parameters{
						Name: "existingstack",
						Mode: modes.ModeAffordable,
					},
				},
			},
			existingStack: &stacks.Parameters{
				Name:     "existingstack",
				Provider: client.ProviderAWS,
				Mode:     modes.ModeBalanced,
			},
			expectedStack: &stacks.Parameters{
				Name:     "existingstack",
				Provider: client.ProviderAWS,
				Mode:     modes.ModeAffordable,
				Variables: map[string]string{
					"DEFANG_PROVIDER": "aws",
					"DEFANG_MODE":     "affordable",
				},
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

			if tt.existingStack == nil {
				if tt.options.Default.Name != "" {
					// For specified non-existing stack, return ErrNotExist
					sm.On("GetStack", ctx, mock.Anything).Maybe().Return(nil, "", &stacks.ErrNotExist{StackName: tt.options.Default.Name})
				} else if tt.getStackError != nil {
					sm.On("GetStack", ctx, mock.Anything).Maybe().Return(nil, "", tt.getStackError)
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
			require.NotNil(t, session)

			// Verify session contents
			require.NotNil(t, session.Loader)

			require.NotNil(t, session.Provider)
			if tt.options.Default.Provider == client.ProviderAWS {
				_, ok := session.Provider.(*aws.ByocAws)
				assert.True(t, ok)
			}
			if tt.options.Default.Provider == client.ProviderGCP {
				_, ok := session.Provider.(*gcp.ByocGcp)
				assert.True(t, ok)
			}

			require.NotNil(t, session.Stack)
			assert.Equal(t, tt.expectedStack.Name, session.Stack.Name)
			assert.Equal(t, tt.expectedStack.Provider, session.Stack.Provider)
			assert.Equal(t, tt.expectedStack.Mode, session.Stack.Mode)

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
