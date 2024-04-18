package cli

import (
	"context"
	"time"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
)

func BootstrapCommand(ctx context.Context, client client.Client, command string) error {
	term.Debug(" - Running CD command", command)
	if DoDryRun {
		return ErrDryRun
	}
	since := time.Now()
	etag, err := client.BootstrapCommand(ctx, command)
	if err != nil || etag == "" {
		return err
	}
	return Tail(ctx, client, "", etag, since, false)
}

func BootstrapList(ctx context.Context, client client.Client) error {
	term.Debug(" - Running CD list")
	if DoDryRun {
		return ErrDryRun
	}
	return client.BootstrapList(ctx)
}
