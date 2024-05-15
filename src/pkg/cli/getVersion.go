package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func GetVersion(ctx context.Context, client client.Client) (string, error) {
	versions, err := client.GetVersions(ctx)
	if err != nil {
		return "", err
	}
	return versions.Fabric, nil
}
