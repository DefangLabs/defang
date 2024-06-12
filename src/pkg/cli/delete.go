package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// Deprecated: Use ComposeStop instead.
func Delete(ctx context.Context, client client.Client, names ...string) (types.ETag, error) {
	term.Debug("Deleting service", names)

	if DoDryRun {
		return "", ErrDryRun
	}

	resp, err := client.Delete(ctx, &defangv1.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
