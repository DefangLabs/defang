package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func GetVersion(ctx context.Context, client client.FabricClient) (string, error) {
	versions, err := client.GetVersions(ctx)
	if err != nil {
		return "", err
	}
	return versions.Fabric, nil
}
