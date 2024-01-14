package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func BootstrapDestroy(ctx context.Context, client client.Client) error {
	if DoDryRun {
		return errors.New("dry run")
	}
	Warn(" ! Destroying all resources")
	return client.Destroy(ctx)
}

func BootstrapRefresh(ctx context.Context, client client.Client) error {
	if DoDryRun {
		return errors.New("dry run")
	}
	Info(" * Refreshing all resources")
	return client.Refresh(ctx)
}
