package cli

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func TestWorkspaceRows(t *testing.T) {
	info := &auth.UserInfo{
		AllTenants: []auth.WorkspaceInfo{
			{ID: "ws-1", Name: "Workspace One"},
			{ID: "ws-2", Name: "Workspace Two"},
		},
	}

	tests := []struct {
		name            string
		info            *auth.UserInfo
		selection       types.TenantNameOrID
		wantCurrentName string
	}{
		{
			name:      "nil info returns nil",
			info:      nil,
			selection: types.TenantUnset,
		},
		{
			name:            "select by ID",
			info:            info,
			selection:       types.TenantNameOrID("ws-2"),
			wantCurrentName: "Workspace Two",
		},
		{
			name:            "select by name",
			info:            info,
			selection:       types.TenantNameOrID("Workspace One"),
			wantCurrentName: "Workspace One",
		},
		{
			name:      "no selection",
			info:      info,
			selection: types.TenantUnset,
		},
		{
			name:      "selection not in list",
			info:      info,
			selection: types.TenantNameOrID("missing"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := WorkspaceRows(tt.info, tt.selection)
			if tt.info == nil {
				if rows != nil {
					t.Fatalf("expected nil rows, got %v", rows)
				}
				return
			}
			if got := len(rows); got != len(tt.info.AllTenants) {
				t.Fatalf("expected %d rows, got %d", len(tt.info.AllTenants), got)
			}
			var current string
			for _, row := range rows {
				if row.Current {
					current = row.Name
					break
				}
			}
			if current != tt.wantCurrentName {
				t.Fatalf("expected current workspace %q, got %q", tt.wantCurrentName, current)
			}
		})
	}
}

func TestResolveWorkspaceName(t *testing.T) {
	info := &auth.UserInfo{
		AllTenants: []auth.WorkspaceInfo{
			{ID: "ws-1", Name: "Workspace One"},
		},
	}

	tests := []struct {
		name      string
		info      *auth.UserInfo
		selection types.TenantNameOrID
		want      string
	}{
		{"nil info, unset selection", nil, types.TenantUnset, ""},
		{"nil info, selection fallback", nil, types.TenantNameOrID("manual"), "manual"},
		{"match by ID", info, types.TenantNameOrID("ws-1"), "Workspace One"},
		{"match by name", info, types.TenantNameOrID("Workspace One"), "Workspace One"},
		{"selection not in list", info, types.TenantNameOrID("other"), "other"},
		{"unset selection with info", info, types.TenantUnset, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveWorkspaceName(tt.info, tt.selection); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestResolveWorkspaceID(t *testing.T) {
	info := &auth.UserInfo{
		AllTenants: []auth.WorkspaceInfo{
			{ID: "ws-1", Name: "Workspace One"},
			{ID: "ws-2", Name: "Workspace Two"},
		},
	}

	tests := []struct {
		name      string
		info      *auth.UserInfo
		selection types.TenantNameOrID
		want      string
	}{
		{"nil info", nil, types.TenantNameOrID("ws-1"), ""},
		{"unset selection", info, types.TenantUnset, ""},
		{"match by ID", info, types.TenantNameOrID("ws-1"), "ws-1"},
		{"match by name", info, types.TenantNameOrID("Workspace Two"), "ws-2"},
		{"selection not in list", info, types.TenantNameOrID("missing"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveWorkspaceID(tt.info, tt.selection); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
