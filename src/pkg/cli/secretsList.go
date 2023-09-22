package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

func SecretsList(ctx context.Context, client defangv1connect.FabricControllerClient) error {
	secrets, err := client.ListSecrets(ctx, &connect.Request[emptypb.Empty]{})
	if err != nil {
		return err
	}

	return PrintObject("", secrets.Msg)
}
