package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

// Deprecated: Use ComposeStop instead.
func Delete(ctx context.Context, client client.Client, names ...string) (client.ETag, error) {
	Debug(" - Deleting service", names)

	if DoDryRun {
		return "", nil
	}

	resp, err := client.Delete(ctx, &v1.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
