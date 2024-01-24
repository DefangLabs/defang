package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func Delete(ctx context.Context, client client.Client, names ...string) (string, error) {
	Debug(" - Deleting service", names)

	if DoDryRun {
		return "", nil
	}

	resp, err := client.Delete(ctx, &pb.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
