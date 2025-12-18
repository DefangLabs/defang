package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ShowAccountData struct {
	Provider       client.ProviderID         `json:"provider"`
	SubscriberTier defangv1.SubscriptionTier `json:"subscriberTier"`
	Region         string                    `json:"region"`
	Workspace      string                    `json:"workspace"`
	Tenant         string                    `json:"tenant,omitempty"` // this is the subdomain
	TenantID       string                    `json:"tenantId,omitempty"`
	Email          string                    `json:"email"`
	Name           string                    `json:"name"`
}

func Whoami(ctx context.Context, fabric client.FabricClient, provider client.Provider, userInfo *auth.UserInfo, tenantSelection types.TenantNameOrID) (ShowAccountData, error) {
	resp, err := fabric.WhoAmI(ctx)
	if err != nil {
		return ShowAccountData{}, err
	}

	term.Debug("User ID: " + resp.UserId)
	showData := ShowAccountData{
		Region:         resp.Region,
		SubscriberTier: resp.Tier,
		Tenant:         resp.Tenant,
		TenantID:       resp.TenantId,
		Workspace:      ResolveWorkspaceName(userInfo, tenantSelection),
	}

	if provider != nil {
		// Add provider account information
		account, err := provider.AccountInfo(ctx)
		if err == nil {
			showData.Provider = account.Provider
			showData.Region = account.Region
		}
	}

	if userInfo != nil {
		// Add information from userinfo
		showData.Email = userInfo.User.Email
		showData.Name = userInfo.User.Name
		if tenantId := ResolveWorkspaceID(userInfo, tenantSelection); tenantId != "" {
			showData.TenantID = tenantId
		}
	}

	return showData, nil
}
