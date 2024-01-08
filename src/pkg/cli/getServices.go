package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func GetServices(ctx context.Context, client client.Client) error {
	serviceList, err := client.GetServices(ctx)
	if err != nil {
		return err
	}

	PrintObject("", serviceList)
	return nil
}
