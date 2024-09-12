package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

// Deprecated: this doesn't do the right thing right now
func Restart(ctx context.Context, client client.Client, names ...string) (types.ETag, error) {
	term.Debug("Restarting service", names)

	if DoDryRun {
		return "", ErrDryRun
	}

	return client.Restart(ctx, names...)
}
