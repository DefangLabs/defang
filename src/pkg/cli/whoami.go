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
	Tenant         string                    `json:"tenant,omitempty"`
	TenantID       string                    `json:"tenantId,omitempty"`
	Email          string                    `json:"email"`
	Name           string                    `json:"name"`
}

func Whoami(ctx context.Context, fabric client.FabricClient, provider client.Provider, userInfo *auth.UserInfo, tenantSelection types.TenantNameOrID, includeTenantDetails bool) (ShowAccountData, error) {
	resp, err := fabric.WhoAmI(ctx)
	if err != nil {
		return ShowAccountData{}, err
	}

	term.Debug("User ID: " + resp.UserId)
	showData := ShowAccountData{
		SubscriberTier: resp.Tier,
		Region:         resp.Region,
		Workspace:      ResolveWorkspaceName(userInfo, tenantSelection),
	}
	if includeTenantDetails {
		showData.Tenant = resp.Tenant
		showData.TenantID = ResolveWorkspaceID(userInfo, tenantSelection)
		if showData.TenantID == "" && resp.TenantId != "" {
			showData.TenantID = resp.TenantId
		}
	}

	if provider != nil {
		account, err := provider.AccountInfo(ctx)
		if err != nil {
			return ShowAccountData{}, err
		}
		showData.Provider = account.Provider
		if account.Region != "" {
			showData.Region = account.Region
		}
	}

	if userInfo != nil {
		showData.Email = userInfo.User.Email
		showData.Name = userInfo.User.Name
	}

	return showData, nil
}
