package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func Restart(ctx context.Context, client client.Client, names ...string) error {
	Debug(" - Restarting service", names)

	if DoDryRun {
		return ErrDryRun
	}

	return client.Restart(ctx, names...)
}
