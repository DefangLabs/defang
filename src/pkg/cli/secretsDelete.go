package cli

import (
	"context"

	"github.com/bufbuild/connect-go"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func SecretsDelete(ctx context.Context, client defangv1connect.FabricControllerClient, name string) error {
	Debug(" - Deleting secret", name)

	if DoDryRun {
		return nil
	}

	_, err := client.PutSecret(ctx, connect.NewRequest(&pb.SecretValue{Name: name}))
	return err
}
