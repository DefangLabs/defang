package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
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

type mockStacksRemover struct {
	mock.Mock
}

func (m *mockStacksRemover) DeleteStack(ctx context.Context, req *defangv1.DeleteStackRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *mockStacksRemover) ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	resp, _ := args.Get(0).(*defangv1.ListDeploymentsResponse)
	return resp, args.Error(1)
}

type mockElicitationsController struct {
	mock.Mock
}

func (m *mockElicitationsController) RequestString(ctx context.Context, message, field string, opts ...func(*elicitations.Options)) (string, error) {
	args := m.Called(ctx, message, field)
	return args.String(0), args.Error(1)
}

func (m *mockElicitationsController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	args := m.Called(ctx, message, field, options)
	return args.String(0), args.Error(1)
}

func (m *mockElicitationsController) SetSupported(supported bool) {
	m.Called(supported)
}

func (m *mockElicitationsController) IsSupported() bool {
	args := m.Called()
	return args.Bool(0)
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

func TestRemoveStack(t *testing.T) {
	ctx := context.Background()

	noDeploymentsResp := &defangv1.ListDeploymentsResponse{}
	downDeploymentResp := &defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{
			{Action: defangv1.DeploymentAction_DEPLOYMENT_ACTION_DOWN},
		},
	}
	upDeploymentResp := &defangv1.ListDeploymentsResponse{
		Deployments: []*defangv1.Deployment{
			{Action: defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP},
		},
	}

	t.Run("list deployments error", func(t *testing.T) {
		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(nil, errors.New("network error"))

		err := RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.ErrorContains(t, err, "network error")
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("no deployments deletes without confirmation", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := stacks.CreateInDirectory(".", stacks.Parameters{Name: "mystack", Provider: client.ProviderAWS, Region: "us-east-1", Mode: modes.ModeAffordable})
		assert.NoError(t, err)

		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(noDeploymentsResp, nil)
		remover.On("DeleteStack", ctx, mock.AnythingOfType("*defangv1.DeleteStackRequest")).Return(nil)

		err = RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.NoError(t, err)
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("last deployment is down, deletes without confirmation", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := stacks.CreateInDirectory(".", stacks.Parameters{Name: "mystack", Provider: client.ProviderAWS, Region: "us-east-1", Mode: modes.ModeAffordable})
		assert.NoError(t, err)

		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(downDeploymentResp, nil)
		remover.On("DeleteStack", ctx, mock.AnythingOfType("*defangv1.DeleteStackRequest")).Return(nil)

		err = RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.NoError(t, err)
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("last deployment is up, non-interactive returns error", func(t *testing.T) {
		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(upDeploymentResp, nil)
		ec.On("IsSupported").Return(false)

		err := RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.ErrorContains(t, err, "re-run in interactive mode")
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("last deployment is up, user declines", func(t *testing.T) {
		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(upDeploymentResp, nil)
		ec.On("IsSupported").Return(true)
		ec.On("RequestEnum", ctx, mock.AnythingOfType("string"), "confirm", []string{"yes", "no"}).Return("no", nil)

		err := RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.ErrorContains(t, err, "cancelled")
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("last deployment is up, user confirms", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := stacks.CreateInDirectory(".", stacks.Parameters{Name: "mystack", Provider: client.ProviderAWS, Region: "us-east-1", Mode: modes.ModeAffordable})
		assert.NoError(t, err)

		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(upDeploymentResp, nil)
		ec.On("IsSupported").Return(true)
		ec.On("RequestEnum", ctx, mock.AnythingOfType("string"), "confirm", []string{"yes", "no"}).Return("yes", nil)
		remover.On("DeleteStack", ctx, mock.AnythingOfType("*defangv1.DeleteStackRequest")).Return(nil)

		err = RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.NoError(t, err)
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("delete stack remote error", func(t *testing.T) {
		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(noDeploymentsResp, nil)
		remover.On("DeleteStack", ctx, mock.AnythingOfType("*defangv1.DeleteStackRequest")).Return(errors.New("dynamo error"))

		err := RemoveStack(ctx, remover, ec, "my-project", "mystack")
		assert.ErrorContains(t, err, "dynamo error")
		remover.AssertExpectations(t)
		ec.AssertExpectations(t)
	})

	t.Run("passes correct project and stack to DeleteStack", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := stacks.CreateInDirectory(".", stacks.Parameters{Name: "beta", Provider: client.ProviderAWS, Region: "us-east-1", Mode: modes.ModeAffordable})
		assert.NoError(t, err)

		remover := &mockStacksRemover{}
		ec := &mockElicitationsController{}
		remover.On("ListDeployments", ctx, mock.AnythingOfType("*defangv1.ListDeploymentsRequest")).Return(noDeploymentsResp, nil)
		remover.On("DeleteStack", ctx, &defangv1.DeleteStackRequest{Project: "acme", Stack: "beta"}).Return(nil)

		err = RemoveStack(ctx, remover, ec, "acme", "beta")
		assert.NoError(t, err)
		remover.AssertExpectations(t)
	})
}
