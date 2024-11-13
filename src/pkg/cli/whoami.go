package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ShowAccountData struct {
	Provider       string
	Details        string
	Region         string
	SubscriberTier string
	Tenant         string
}

func showAccountInfo(showData ShowAccountData) (string, error) {
	if showData.Provider == "" {
		showData.Provider = "Defang"
	}
	outputText := "WhoAmI - \n\tProvider: " + showData.Provider

	if showData.Tenant != "" {
		outputText += "\n\tTenant: " + showData.Tenant
	}

	if showData.SubscriberTier != pkg.SubscriptionTierToString(defangv1.SubscriptionTier_SUBSCRIPTION_TIER_UNSPECIFIED) {
		outputText += "\n\tSubscription: " + showData.SubscriberTier
	}

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

	account, err := provider.AccountInfo(ctx)
	if err != nil {
		return "", err
	}

	showData.Provider = resp.Account
	if account.AccountID() != "" {
		showData.Provider = account.AccountID()
	}

	showData.Region = resp.Region
	if account.Region() != "" {
		showData.Region = account.Region()
	}

	showData.Tenant = resp.Tenant
	showData.Details = account.Details()

	showData.SubscriberTier = pkg.SubscriptionTierToString(defangv1.SubscriptionTier_SUBSCRIPTION_TIER_UNSPECIFIED)
	if account.SubscriptionTier() != defangv1.SubscriptionTier_SUBSCRIPTION_TIER_UNSPECIFIED {
		showData.SubscriberTier = pkg.SubscriptionTierToString(account.SubscriptionTier())
	}

	return showAccountInfo(showData)
}
