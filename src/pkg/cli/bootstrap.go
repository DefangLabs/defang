package cli

import (
	"context"
	"time"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func BootstrapCommand(ctx context.Context, client client.Client, command string, timezone *time.Location, timeFormat string) error {
	Debug(" - Running CD command", command)
	if DoDryRun {
		return ErrDryRun
	}

	since := time.Now()
	etag, err := client.BootstrapCommand(ctx, command)
	if err != nil || etag == "" {
		return err
	}
	return Tail(ctx, client, LogDisplayArgs{
		Service:    "",
		Etag:       etag,
		Since:      since,
		Raw:        false,
		TimeZone:   timezone,
		TimeFormat: timeFormat,
	})
}

func BootstrapList(ctx context.Context, client client.Client) error {
	Debug(" - Running CD list")
	if DoDryRun {
		return ErrDryRun
	}
	return client.BootstrapList(ctx)
}
