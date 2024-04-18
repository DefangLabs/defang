package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func SecretsSet(ctx context.Context, client client.Client, name string, value string) error {
	Debug(" - Setting secret", name)

	if value == "" {
		return errors.New("value cannot be empty") // FIXME: remove once we implement DeleteSecrets rpc
	}

	if DoDryRun {
		return ErrDryRun
	}

	err := client.PutSecret(ctx, &defangv1.SecretValue{Name: name, Value: value})
	return err
}
