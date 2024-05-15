package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
)

func Whoami(ctx context.Context, client client.Client) error {
	resp, err := client.WhoAmI(ctx)
	if err != nil {
		return err
	}
	term.Infof(" * You are logged into tenant %q in %q region %q", resp.Tenant, resp.Account, resp.Region)
	return nil
}
