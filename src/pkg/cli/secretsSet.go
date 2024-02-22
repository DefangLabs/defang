package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func SecretsSet(ctx context.Context, client client.Client, name string, value string) error {
	Debug(" - Setting secret", name)

	if value == "" {
		return errors.New("value cannot be empty")
	}

	if DoDryRun {
		return nil
	}

	err := client.PutSecret(ctx, &v1.SecretValue{Name: name, Value: value})
	return err
}
