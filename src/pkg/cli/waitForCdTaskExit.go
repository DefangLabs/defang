package cli

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

var pollDuration = 2 * time.Second

func WaitForCdTaskExit(ctx context.Context, provider client.Provider) error {
	ticker := time.NewTicker(pollDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			done, err := provider.GetDeploymentStatus(ctx)
			// End condition: EOF indicates that the task has completed successfully
			if done || errors.Is(err, io.EOF) {
				return nil
			}
			// Retry on transient errors
			if isTransientError(err) {
				// If it's a transient error, we can retry at the next tick
				continue
			}
			if err != nil {
				return err
			}
			// nil means the task is still running and we continue polling
		case <-ctx.Done(): // Stop the loop when the context is cancelled
			return ctx.Err()
		}
	}
}
