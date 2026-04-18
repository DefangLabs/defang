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
			// slog.Debug(fmt.Sprintf("Polled CD task status: done=%v, err=%v", done, err))
			if err != nil {
				// End condition: EOF indicates that the task has completed successfully
				if errors.Is(err, io.EOF) {
					return nil
				}
				// Retry on transient errors
				if isTransientError(err) {
					// If it's a transient error, we can retry at the next tick
					continue
				}
				return err
			}
			if done {
				return nil
			}
			// the task is still running and we continue polling
		case <-ctx.Done(): // Stop the loop when the context is cancelled
			return ctx.Err()
		}
	}
}
