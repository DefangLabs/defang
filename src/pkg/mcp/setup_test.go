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
		client        string
		goos          string
		appData       string
		xdgConfigHome string
		expectedPath  string
		expectedError bool
	}{
		// Windsurf/Cascade/Codeium tests
		{
			name:         "windsurf",
			client:       "windsurf",
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:         "cascade",
			client:       "cascade",
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:         "codeium",
			client:       "codeium",
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},

		// Claude tests - Darwin
		{
			name:         "claude_darwin",
			client:       "claude",
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Windows with APPDATA
		{
			name:         "claude_windows_with_appdata",
			client:       "claude",
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Windows without APPDATA
		{
			name:         "claude_windows_without_appdata",
			client:       "claude",
			goos:         "windows",
			appData:      "",
			expectedPath: filepath.Join(homeDir, "AppData", "Roaming", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Linux with XDG_CONFIG_HOME
		{
			name:          "claude_linux_with_xdg",
			client:        "claude",
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Claude", "claude_desktop_config.json"),
		},

		// Claude tests - Linux without XDG_CONFIG_HOME
		{
			name:          "claude_linux_without_xdg",
			client:        "claude",
			goos:          "linux",
			xdgConfigHome: "",
			expectedPath:  filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json"),
		},

		// Cursor tests
		{
			name:         "cursor",
			client:       "cursor",
			expectedPath: filepath.Join(homeDir, ".cursor", "mcp.json"),
		},

		// VSCode tests - Darwin
		{
			name:         "vscode_darwin",
			client:       "vscode",
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json"),
		},
		{
			name:         "code_darwin",
			client:       "code",
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json"),
		},

		// VSCode tests - Windows with APPDATA
		{
			name:         "vscode_windows_with_appdata",
			client:       "vscode",
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Code", "User", "settings.json"),
		},

		// VSCode tests - Windows without APPDATA
		{
			name:         "vscode_windows_without_appdata",
			client:       "vscode",
			goos:         "windows",
			appData:      "",
			expectedPath: filepath.Join(homeDir, "AppData", "Roaming", "Code", "User", "settings.json"),
		},

		// VSCode tests - Linux with XDG_CONFIG_HOME
		{
			name:          "vscode_linux_with_xdg",
			client:        "vscode",
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Code/User/settings.json"),
		},

		// VSCode tests - Linux without XDG_CONFIG_HOME
		{
			name:          "vscode_linux_without_xdg",
			client:        "vscode",
			goos:          "linux",
			xdgConfigHome: "",
			expectedPath:  filepath.Join(homeDir, ".config", "Code/User/settings.json"),
		},

		// VSCode Insiders tests - Darwin
		{
			name:         "vscode_insiders_darwin",
			client:       "vscode-insiders",
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "settings.json"),
		},
		{
			name:         "insiders_darwin",
			client:       "insiders",
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "settings.json"),
		},

		// VSCode Insiders tests - Windows with APPDATA
		{
			name:         "vscode_insiders_windows_with_appdata",
			client:       "vscode-insiders",
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Code - Insiders", "User", "settings.json"),
		},

		// VSCode Insiders tests - Linux with XDG_CONFIG_HOME
		{
			name:          "vscode_insiders_linux_with_xdg",
			client:        "vscode-insiders",
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Code - Insiders/User/settings.json"),
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
