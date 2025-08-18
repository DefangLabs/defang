package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ShowAccountData struct {
	client.AccountInfo
	SubscriberTier defangv1.SubscriptionTier
	Tenant         string
}

func showAccountInfo(showData ShowAccountData) (string, error) {
	if showData.Provider == "" {
		showData.Provider = "Defang"
	}
	outputText := "WhoAmI - \n\tProvider: " + showData.Provider.Name()

	if showData.AccountID != "" {
		outputText += "\n\tAccountID: " + showData.AccountID
	}

	if showData.Tenant != "" {
		outputText += "\n\tTenant: " + showData.Tenant
	}

	outputText += "\n\tSubscription Tier: " + pkg.SubscriptionTierToString(showData.SubscriberTier)

	if showData.Region != "" {
		outputText += "\n\tRegion: " + showData.Region
	}

	if showData.Details != "" {
		outputText += "\n\tDetails: " + showData.Details
	}

	return outputText, nil
}

func Whoami(ctx context.Context, fabric client.FabricClient, provider client.Provider) (string, error) {
	showData := ShowAccountData{}

	resp, err := fabric.WhoAmI(ctx)
	if err != nil {
		return "", err
	}

	term.Debug("User ID: " + resp.UserId)
	showData.Region = resp.Region
	showData.SubscriberTier = resp.Tier
	showData.Tenant = resp.Tenant

	if provider != nil {
		account, err := provider.AccountInfo(ctx)
		if err != nil {
			return "", err
		}
		showData.AccountInfo = *account
	}

	return showAccountInfo(showData)
}
