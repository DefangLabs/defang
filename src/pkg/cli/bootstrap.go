package cli

import (
	"context"
	"errors"
	"time"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func BootstrapCommand(ctx context.Context, client client.Client, command string) error {
	Debug(" - Running CD command", command)
	if DoDryRun {
		return errors.New("dry run")
	}
	since := time.Now()
	etag, err := client.BootstrapCommand(ctx, command)
	if err != nil || etag == "" {
		return err
	}
	return Tail(ctx, client, "", etag, since, false)
}
