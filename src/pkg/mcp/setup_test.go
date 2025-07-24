package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetClientConfigPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name          string
		client        MCPClient
		goos          string
		appData       string
		xdgConfigHome string
		expectedPath  string
		expectedError bool
	}{
		// Windsurf/Cascade/Codeium tests
		{
			name:         "windsurf",
			client:       MCPClientWindsurf,
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:         "cascade",
			client:       MCPClientCascade,
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:         "codeium",
			client:       MCPClientCodeium,
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},

		// Claude tests - Darwin
		{
			name:         "claude_darwin",
			client:       MCPClientClaude,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Windows with APPDATA
		{
			name:         "claude_windows_with_appdata",
			client:       MCPClientClaude,
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Windows without APPDATA
		{
			name:         "claude_windows_without_appdata",
			client:       MCPClientClaude,
			goos:         "windows",
			appData:      "",
			expectedPath: filepath.Join(homeDir, "AppData", "Roaming", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Linux with XDG_CONFIG_HOME
		{
			name:          "claude_linux_with_xdg",
			client:        MCPClientClaude,
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Linux without XDG_CONFIG_HOME
		{
			name:          "claude_linux_without_xdg",
			client:        MCPClientClaude,
			goos:          "linux",
			xdgConfigHome: "",
			expectedPath:  filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json"),
		},

		// Cursor tests
		{
			name:         "cursor",
			client:       MCPClientCursor,
			expectedPath: filepath.Join(homeDir, ".cursor", "settings.json"),
		},

		// VSCode tests - Darwin
		{
			name:         "vscode_darwin",
			client:       MCPClientVSCode,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "mcp.json"),
		},
		{
			name:         "code_darwin",
			client:       MCPClientCode,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "mcp.json"),
		},

		// VSCode tests - Windows with APPDATA
		{
			name:         "vscode_windows_with_appdata",
			client:       MCPClientVSCode,
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Code", "User", "mcp.json"),
		},

		// VSCode tests - Windows without APPDATA
		{
			name:         "vscode_windows_without_appdata",
			client:       MCPClientVSCode,
			goos:         "windows",
			appData:      "",
			expectedPath: filepath.Join(homeDir, "AppData", "Roaming", "Code", "User", "mcp.json"),
		},

		// VSCode tests - Linux with XDG_CONFIG_HOME
		{
			name:          "vscode_linux_with_xdg",
			client:        MCPClientVSCode,
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Code/User/mcp.json"),
		},

		// VSCode tests - Linux without XDG_CONFIG_HOME
		{
			name:          "vscode_linux_without_xdg",
			client:        MCPClientVSCode,
			goos:          "linux",
			xdgConfigHome: "",
			expectedPath:  filepath.Join(homeDir, ".config", "Code/User/mcp.json"),
		},

		// VSCode Insiders tests - Darwin
		{
			name:         "vscode_insiders_darwin",
			client:       MCPClientVSCodeInsiders,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "mcp.json"),
		},
		{
			name:         "insiders_darwin",
			client:       MCPClientInsiders,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "mcp.json"),
		},

		// VSCode Insiders tests - Windows with APPDATA
		{
			name:         "vscode_insiders_windows_with_appdata",
			client:       MCPClientVSCodeInsiders,
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Code - Insiders", "User", "mcp.json"),
		},

		// VSCode Insiders tests - Linux with XDG_CONFIG_HOME
		{
			name:          "vscode_insiders_linux_with_xdg",
			client:        MCPClientVSCodeInsiders,
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Code - Insiders/User/mcp.json"),
		},

		// Error cases
		{
			name:          "unsupported_client",
			client:        "unsupported",
			expectedError: true,
		},
		{
			name:          "empty_client",
			client:        "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.appData != "" {
				t.Setenv("APPDATA", tt.appData)
			}

			if tt.xdgConfigHome != "" {
				t.Setenv("XDG_CONFIG_HOME", tt.xdgConfigHome)
			}

			configPath, err := getClientConfigPath(homeDir, tt.goos, tt.client)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error for client %s, but got none", tt.client)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for client %s: %v", tt.client, err)
				} else if configPath != tt.expectedPath {
					t.Errorf("Expected path %s for client %s, but got %s", tt.expectedPath, tt.client, configPath)
				}
			}
		})
	}
}
