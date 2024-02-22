package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func GetVersion(ctx context.Context, client client.Client) (string, error) {
	status, err := client.GetVersion(ctx)
	if err != nil {
		return "", err
	}
	return status.Fabric, nil
}
