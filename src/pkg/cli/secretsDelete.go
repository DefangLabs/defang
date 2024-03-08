package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func SecretsDelete(ctx context.Context, client client.Client, names ...string) error {
	Debug(" - Deleting secret", names)

	if DoDryRun {
		return ErrDryRun
	}

	return client.DeleteSecrets(ctx, &v1.Secrets{Names: names})
}
