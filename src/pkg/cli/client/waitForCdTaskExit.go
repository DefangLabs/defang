package client

import (
	"context"
	"errors"
	"io"
	"time"
)

var pollDuration = 2 * time.Second

func WaitForCdTaskExit(ctx context.Context, provider Provider) error {
	ticker := time.NewTicker(pollDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := provider.GetDeploymentStatus(ctx); err != nil {
				if errors.Is(err, io.EOF) {
					// EOF indicates that the task has completed successfully
					return nil
				}
				return err
			}
		case <-ctx.Done(): // Stop the loop when the context is cancelled
			return ctx.Err()
		}
	}
}
