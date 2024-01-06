package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func Delete(ctx context.Context, client client.Client, name ...string) (string, error) {
	Debug(" - Deleting service", name)

	if DoDryRun {
		return "", nil
	}

	for i, n := range name {
		name[i] = NormalizeServiceName(n)
	}
	resp, err := client.Delete(ctx, &pb.DeleteRequest{Names: name})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
