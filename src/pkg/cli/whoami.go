package cli

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func Whoami(ctx context.Context, client cliClient.Client) error {
	resp, err := client.WhoAmI(ctx)
	if err != nil {
		return err
	}

	term.Infof("You are logged into %s region %s with tenant %q",
		resp.Account, resp.Region, resp.Tenant)

	return nil
}
