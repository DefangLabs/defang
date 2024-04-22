package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func SecretsDelete(ctx context.Context, client client.Client, names ...string) error {
	term.Debug(" - Deleting secret", names)

	if DoDryRun {
		return ErrDryRun
	}

	return client.DeleteSecrets(ctx, &defangv1.Secrets{Names: names})
}
