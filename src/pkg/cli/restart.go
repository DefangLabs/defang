package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
)

func Restart(ctx context.Context, client client.Client, names ...string) error {
	term.Debug(" - Restarting service", names)

	if DoDryRun {
		return ErrDryRun
	}

	return client.Restart(ctx, names...)
}
