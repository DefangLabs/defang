package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func TearDown(ctx context.Context, client client.Client) error {
	if DoDryRun {
		return errors.New("dry run")
	}
	return client.TearDown(ctx)
}
