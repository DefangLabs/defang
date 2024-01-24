package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func BootstrapCommand(ctx context.Context, client client.Client, command string) error {
	if DoDryRun {
		return errors.New("dry run")
	}
	return client.BootstrapCommand(ctx, command)
}
