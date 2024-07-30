package cli

import (
	"context"
	"fmt"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

func Whoami(ctx context.Context, client cliClient.Client) (string, error) {
	resp, err := client.WhoAmI(ctx)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"You are logged into %s region %s with tenant %q",
		resp.Account, resp.Region, resp.Tenant,
	), nil
}
