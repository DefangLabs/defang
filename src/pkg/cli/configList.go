package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func ConfigList(ctx context.Context, client client.Client) error {
	config, err := client.ListConfig(ctx)
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
