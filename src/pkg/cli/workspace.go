package cli

import (
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/types"
)

type WorkspaceRow struct {
	Name    string `json:"name"`
	ID      string `json:"id,omitempty"`
	Current bool   `json:"current"`
}

// WorkspaceRows marks the current workspace using tenant selection and optional WhoAmI tenant data.
// The caller can pass an empty currentTenantID when only token/flag selection is available.
func WorkspaceRows(info *auth.UserInfo, tenantSelection types.TenantNameOrID, currentTenantID string) []WorkspaceRow {
	if info == nil {
		return nil
	}

	currentWorkspace := tenantSelection
	if !currentWorkspace.IsSet() && currentTenantID != "" {
		currentWorkspace = types.TenantNameOrID(currentTenantID)
	}

	rows := make([]WorkspaceRow, 0, len(info.AllTenants))
	for _, t := range info.AllTenants {
		isCurrent := false
		if currentTenantID != "" && (t.ID == currentTenantID || t.Name == currentTenantID) {
			isCurrent = true
		} else if currentWorkspace.IsSet() && (t.ID == string(currentWorkspace) || t.Name == string(currentWorkspace)) {
			isCurrent = true
		}
		rows = append(rows, WorkspaceRow{
			Name:    t.Name,
			ID:      t.ID,
			Current: isCurrent,
		})
	}

	return rows
}

// ResolveWorkspaceName maps the selected tenant (flag/env/token subject) to a known workspace name when available.
// If no selection is set, returns an empty string.
func ResolveWorkspaceName(info *auth.UserInfo, tenantSelection types.TenantNameOrID) string {
	if info == nil {
		if tenantSelection.IsSet() {
			return string(tenantSelection)
		}
		return ""
	}

	for _, t := range info.AllTenants {
		if tenantSelection.IsSet() && (t.ID == string(tenantSelection) || t.Name == string(tenantSelection)) {
			return t.Name
		}
	}

	if tenantSelection.IsSet() {
		return string(tenantSelection)
	}

	return ""
}

// ResolveWorkspaceID returns the workspace ID matching the provided selection (flag/env/token subject).
// If no match is found, returns an empty string.
func ResolveWorkspaceID(info *auth.UserInfo, tenantSelection types.TenantNameOrID) string {
	if info == nil || !tenantSelection.IsSet() {
		return ""
	}
	for _, t := range info.AllTenants {
		if t.ID == string(tenantSelection) || t.Name == string(tenantSelection) {
			return t.ID
		}
	}
	return ""
}
