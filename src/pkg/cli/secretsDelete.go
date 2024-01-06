package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func SecretsDelete(ctx context.Context, client client.Client, name string) error {
	Debug(" - Deleting secret", name)

	if DoDryRun {
		return nil
	}

	err := client.PutSecret(ctx, &pb.SecretValue{Name: name})
	return err
}
