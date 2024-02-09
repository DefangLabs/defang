package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func SecretsList(ctx context.Context, client client.Client) error {
	secrets, err := client.ListSecrets(ctx)
	if err != nil {
		return err
	}

	return PrintObject("", secrets)
}
