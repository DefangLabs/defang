package cli

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/stretchr/testify/assert"
)

type mockCdWaiter struct {
	client.MockProvider
	getDeploymentStatusErr error
}

func (m *mockCdWaiter) GetDeploymentStatus(ctx context.Context) error {
	err := m.getDeploymentStatusErr
	// This logic was copied from AWS provider, to ensure the errs work correctly
	if taskErr := new(ecs.TaskFailure); errors.As(err, taskErr) {
		return client.ErrDeploymentFailed{Message: taskErr.Error()}
	}
	return err
}

func TestWaitForCdTaskExit(t *testing.T) {
	t.Run("ECS task failure", func(t *testing.T) {
		waiter := &mockCdWaiter{
			getDeploymentStatusErr: ecs.TaskFailure{},
		}
		err := WaitForCdTaskExit(t.Context(), waiter)
		assert.ErrorAs(t, err, &client.ErrDeploymentFailed{})
	})

	t.Run("keep running until canceled", func(t *testing.T) {
		waiter := &mockCdWaiter{
			getDeploymentStatusErr: nil,
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*pollDuration)
		t.Cleanup(cancel)
		err := WaitForCdTaskExit(ctx, waiter)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("CD task exit", func(t *testing.T) {
		waiter := &mockCdWaiter{
			getDeploymentStatusErr: io.EOF,
		}
		err := WaitForCdTaskExit(t.Context(), waiter)
		assert.NoError(t, err)
	})

	t.Run("CD task error", func(t *testing.T) {
		waiter := &mockCdWaiter{
			getDeploymentStatusErr: errors.New("some error"),
		}
		err := WaitForCdTaskExit(t.Context(), waiter)
		assert.EqualError(t, err, "some error")
	})
}
