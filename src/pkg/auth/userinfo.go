package auth

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
)

type WorkspaceInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type UserDetails struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type UserInfo struct {
	AllTenants []WorkspaceInfo `json:"allTenants"`
	User       UserDetails     `json:"userinfo"`
}

func (ui *UserInfo) FindWorkspaceInfo(tenantSelection types.TenantNameOrID) *WorkspaceInfo {
	if ui == nil || !tenantSelection.IsSet() {
		return nil
	}
	for _, wi := range ui.AllTenants {
		if wi.ID == string(tenantSelection) || wi.Name == string(tenantSelection) {
			return &wi
		}
	}
	return nil
}

func FetchUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	return OpenAuthClient.UserInfo(ctx, accessToken)
}
