package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func SecretsDelete(ctx context.Context, client client.Client, name string) error {
	Debug(" - Deleting secret", name)

	if DoDryRun {
		return ErrDryRun
	}

	// FIXME: create dedicated DeleteSecret method in client proto
	err := client.PutSecret(ctx, &v1.SecretValue{Name: name})
	return err
}
