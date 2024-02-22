package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func Whoami(ctx context.Context, client client.Client) error {
	resp, err := client.WhoAmI(ctx)
	if err != nil {
		return err
	}
	Info(" * You are logged in to tenant", resp.Tenant, "in", resp.Account, "region", resp.Region)
	return nil
}
