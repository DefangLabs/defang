package cli

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/dryrun"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockConfigManager struct {
	mock.Mock
}

func (m *mockConfigManager) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	args := m.Called(ctx, req)
	secret, ok := args.Get(0).(*defangv1.Secrets)
	if !ok && args.Get(0) != nil {
		return nil, args.Error(1)
	}
	return secret, args.Error(1)
}

func (m *mockConfigManager) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func TestConfigSet(t *testing.T) {
	ctx := t.Context()

	provider := &mockConfigManager{}

	tests := []struct {
		name           string
		configName     string
		existing       []string
		ifNotSet       bool
		expectedError  error
		expectedDidSet bool
	}{
		{
			name:           "config not set, should set",
			configName:     "test",
			existing:       []string{},
			ifNotSet:       true,
			expectedDidSet: true,
		},
		{
			name:           "config already set, should skip",
			configName:     "test",
			existing:       []string{"test"},
			ifNotSet:       true,
			expectedDidSet: false,
		},
		{
			name:           "config already set, should overwrite",
			configName:     "test",
			existing:       []string{"test_name"},
			ifNotSet:       false,
			expectedDidSet: true,
		},
		{
			name:          "invalid config name, should error",
			configName:    "123invalid",
			expectedError: ErrInvalidConfigName{Name: "123invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := "test"
			value := "test_value"
			provider.ExpectedCalls = nil // Reset expectations

			provider.On("ListConfig", mock.Anything, &defangv1.ListConfigsRequest{Project: project}).
				Maybe().
				Return(&defangv1.Secrets{Names: tt.existing}, nil)

			provider.On("PutConfig", mock.Anything, &defangv1.PutConfigRequest{
				Project: project,
				Name:    tt.configName,
				Value:   value,
			}).Maybe().Return(nil)

			didSet, err := ConfigSet(ctx, project, provider, tt.configName, value, ConfigSetOptions{
				IfNotSet: tt.ifNotSet,
			})

			if tt.expectedError != nil {
				require.ErrorContains(t, err, tt.expectedError.Error())
			}
			assert.Equal(t, tt.expectedDidSet, didSet)

			provider.AssertExpectations(t)
		})
	}

	t.Run("expect no error", func(t *testing.T) {
		provider.ExpectedCalls = nil // Reset expectations
		provider.On("PutConfig", mock.Anything, &defangv1.PutConfigRequest{
			Project: "test",
			Name:    "test_name",
			Value:   "test_value",
		}).Return(nil)

		_, err := ConfigSet(ctx, "test", provider, "test_name", "test_value", ConfigSetOptions{})
		require.NoError(t, err)
		provider.AssertExpectations(t)
	})

	t.Run("expect error on DryRun", func(t *testing.T) {
		dryrun.DoDryRun = true
		t.Cleanup(func() { dryrun.DoDryRun = false })
		_, err := ConfigSet(ctx, "test", provider, "test_name", "test_value", ConfigSetOptions{})
		require.ErrorIs(t, err, dryrun.ErrDryRun)
	})
}
