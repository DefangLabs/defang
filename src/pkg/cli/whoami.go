package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func Whoami(ctx context.Context, client client.Client) error {
	resp, err := client.WhoAmI(ctx)
	if err != nil {
		return err
	}
	term.Infof(" * You are logged into tenant %q in %q region %q", resp.Tenant, resp.Account, resp.Region)
	return nil
}
