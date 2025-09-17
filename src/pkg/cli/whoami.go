package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ShowAccountData struct {
	client.AccountInfo
	SubscriberTier defangv1.SubscriptionTier
	Tenant         string
	TenantID       string
}

func Whoami(ctx context.Context, fabric client.FabricClient, provider client.Provider) (ShowAccountData, error) {
	showData := ShowAccountData{}

	resp, err := fabric.WhoAmI(ctx)
	if err != nil {
		return ShowAccountData{}, err
	}

	term.Debug("User ID: " + resp.UserId)
	showData.Region = resp.Region
	showData.SubscriberTier = resp.Tier
	showData.Tenant = resp.Tenant
	showData.TenantID = resp.TenantId

	if provider != nil {
		account, err := provider.AccountInfo(ctx)
		if err != nil {
			return ShowAccountData{}, err
		}
		showData.AccountInfo = *account
	}

	return showData, err
}
