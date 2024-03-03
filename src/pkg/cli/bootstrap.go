package cli

import (
	"context"
	"time"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func BootstrapCommand(ctx context.Context, client client.Client, command string) error {
	if DoDryRun {
		return ErrDryRun
	}
	etag, err := client.BootstrapCommand(ctx, command)
	if err != nil || etag == "" {
		return err
	}
	return Tail(ctx, client, "", etag, time.Now(), false)
}
