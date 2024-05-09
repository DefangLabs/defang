package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/types"
)

func ComposeDown(ctx context.Context, client client.Client) (types.ETag, error) {
	return client.Destroy(ctx)
}
