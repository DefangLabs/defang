package cli

import (
	"context"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func WaitCdTaskState(
	ctx context.Context,
	provider client.Provider,
) error {
	term.Debug("waiting for cdTask to complete.\n") // TODO: don't print in Go-routine

	if DoDryRun {
		return ErrDryRun
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
