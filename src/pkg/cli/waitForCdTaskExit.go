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
			if err := provider.GetDeploymentStatus(ctx); err != nil {
				if errors.Is(err, io.EOF) {
					// EOF indicates that the task has completed successfully
					return nil
				} else if isTransientError(err) {
					// If it's a transient error, we can retry
					if err := provider.DelayBeforeRetry(ctx); err != nil {
						return err
					}
					continue
				}
				return err
			}
		case <-ctx.Done(): // Stop the loop when the context is cancelled
			return ctx.Err()
		}
	}
}
