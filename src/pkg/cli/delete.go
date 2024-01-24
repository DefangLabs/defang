package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func Delete(ctx context.Context, client defangv1connect.FabricControllerClient, names ...string) (string, error) {
	Debug(" - Deleting service", names)

	if DoDryRun {
		return "", nil
	}

	resp, err := client.Delete(ctx, connect.NewRequest(&pb.DeleteRequest{Names: names}))
	if err != nil {
		return "", err
	}
	return resp.Msg.Etag, nil
}
