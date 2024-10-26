package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func Whoami(ctx context.Context, provider client.Provider) (string, error) {
	resp, err := provider.WhoAmI(ctx)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"You are logged into %s region %s with tenant %q",
		resp.Account, resp.Region, resp.Tenant,
	), nil
}
