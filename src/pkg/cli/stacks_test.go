package cli

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockStacksLoader struct {
	mock.Mock
}

func (m *mockStacksLoader) Load(ctx context.Context, name string) (*stacks.Parameters, error) {
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

type mockStacksPutter struct {
	mock.Mock
}

func (m *mockStacksPutter) PutStack(ctx context.Context, req *defangv1.PutStackRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func TestSetDefaultStack(t *testing.T) {
	type testCase struct {
		name        string
		stackName   string
		projectName string
		loadReturn  *stacks.Parameters
		loadErr     error
		putErr      error
		expectErr   bool
	}

	ctx := context.Background()

	tests := []testCase{
		{
			name:        "success",
			stackName:   "test-stack",
			projectName: "test-project",
			loadReturn: &stacks.Parameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			loadErr:   nil,
			putErr:    nil,
			expectErr: false,
		},
		{
			name:        "load error",
			stackName:   "foo",
			projectName: "test-project",
			loadReturn:  nil,
			loadErr:     assert.AnError,
			putErr:      nil,
			expectErr:   true,
		},
		{
			name:        "put error",
			stackName:   "bar",
			projectName: "test-project",
			loadReturn: &stacks.Parameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			loadErr:   nil,
			putErr:    assert.AnError,
			expectErr: true,
		},
		{
			name:        "empty stack name",
			stackName:   "",
			projectName: "test-project",
			loadReturn:  nil,
			loadErr:     assert.AnError,
			putErr:      nil,
			expectErr:   true,
		},
		{
			name:        "empty project name",
			stackName:   "test-stack",
			projectName: "",
			loadReturn: &stacks.Parameters{
				Name:     "test-stack",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			loadErr:   nil,
			putErr:    nil,
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockStacksLoader := &mockStacksLoader{}
			mockStacksPutter := &mockStacksPutter{}

			mockStacksLoader.On("Load", ctx, tc.stackName).Return(tc.loadReturn, tc.loadErr)
			if tc.loadErr == nil {
				mockStacksPutter.On("PutStack", ctx, mock.AnythingOfType("*defangv1.PutStackRequest")).Return(tc.putErr)
			}

			err := SetDefaultStack(ctx, mockStacksPutter, mockStacksLoader, tc.projectName, tc.stackName)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockStacksLoader.AssertExpectations(t)
			mockStacksPutter.AssertExpectations(t)
		})
	}
}
