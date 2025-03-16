package cli

import (
	"context"
	"errors"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

var ErrNoCdTaskToMonitor = errors.New("no cd taask to monitor")

func WaitCdTaskState(
	ctx context.Context,
	provider client.Provider,
) error {
	term.Debug("waiting for cdTask to complete.\n") // TODO: don't print in Go-routine

	if DoDryRun {
		return ErrDryRun
	}

	ctx, cancel := context.WithCancel(context.Background()) // Create a cancellable context
	defer cancel()                                          // Ensure cancel is called to avoid context leak

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := provider.GetDeploymentStatus(ctx); err != nil {
				return err
			}
		case <-ctx.Done(): // Stop the loop when the context is cancelled
			return ctx.Err()
		}
	}
}
