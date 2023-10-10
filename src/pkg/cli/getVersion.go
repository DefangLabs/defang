package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

func GetVersion(ctx context.Context, client defangv1connect.FabricControllerClient) (string, error) {
	status, err := client.GetVersion(ctx, &connect.Request[emptypb.Empty]{})
	if err != nil {
		return "", err
	}
	return status.Msg.Fabric, nil
}
