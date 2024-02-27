package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func ComposeDown(ctx context.Context, client client.Client) (client.ETag, error) {
	return client.Destroy(ctx)
}
