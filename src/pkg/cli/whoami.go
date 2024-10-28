package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func Whoami(ctx context.Context, fabric client.FabricClient, provider client.Provider) (string, error) {
	resp, err := fabric.WhoAmI(ctx)
	if err != nil {
		return "", err
	}

	account, err := provider.AccountInfo(ctx)
	if err != nil {
		return "", err
	}

	accountID := resp.Account
	if account.AccountID() != "" {
		accountID = account.AccountID()
	}

	region := resp.Region
	if account.Region() != "" {
		region = account.Region()
	}

	if account.Details() != "" {
		accountID += " (" + account.Details() + ")"
	}

	// TODO: Add provider name here?
	return fmt.Sprintf(
		"You are logged into %s region %s with tenant %q",
		accountID, region, resp.Tenant,
	), nil
}
