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

// WorkspaceRows marks the current workspace using the provided tenant selection.
func WorkspaceRows(info *auth.UserInfo, tenantSelection types.TenantNameOrID) []WorkspaceRow {
	if info == nil {
		return nil
	}

	currentWorkspace := tenantSelection
	rows := make([]WorkspaceRow, 0, len(info.AllTenants))
	for _, t := range info.AllTenants {
		isCurrent := false
		if currentWorkspace.IsSet() && (t.ID == string(currentWorkspace) || t.Name == string(currentWorkspace)) {
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
	if wi := info.FindWorkspaceInfo(tenantSelection); wi != nil {
		return wi.Name
	}
	// If we didn't resolve a workspace name, display the raw selection for transparency.
	if tenantSelection.IsSet() {
		return string(tenantSelection)
	}
	return ""
}

// ResolveWorkspaceID returns the workspace ID matching the provided selection (flag/env/token subject).
// If no match is found, returns an empty string.
func ResolveWorkspaceID(info *auth.UserInfo, tenantSelection types.TenantNameOrID) string {
	if wi := info.FindWorkspaceInfo(tenantSelection); wi != nil {
		return wi.ID
	}
	return ""
}
