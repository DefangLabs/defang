package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

func Logout(ctx context.Context, client defangv1connect.FabricControllerClient) error {
	Debug(" - Logging out")
	_, err := client.RevokeToken(ctx, connect.NewRequest(&emptypb.Empty{}))
	// Ignore unauthenticated errors, since we're logging out anyway
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		return err
	}
	// TODO: remove the cached token file
	// tokenFile := getTokenFile(fabric)
	// if err := os.Remove(tokenFile); err != nil {
	// 	return err
	// }
	return nil
}
