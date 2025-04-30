package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// VScodeConfig structure
type VScodeConfig struct {
	MCP struct {
		Servers map[string]VSCodeMCPServerConfig `json:"servers"`
	} `json:"mcp"`
	// Other VSCode settings can be included as needed
	Other map[string]interface{} `json:"-"`
}

func TestSetupClient_TableDriven(t *testing.T) {
	tests := []struct {
		name           string // Test name
		ideClient      string // IDE client name
		clientInstall  bool   // Flag to indicate if the client is installed
		initialConfig  string // Initial configuration to be written to the config file if there is any
		expectedConfig string // Expected configuration after running SetupClient
		expectError    string // Expected error message if any
	}{
		{
			name:          "Misspelled client name",
			ideClient:     "windsrf",
			clientInstall: true,
			expectError:   "invalid MCP client: windsrf.",
		},
		{
			name:          "Unsupported client",
			ideClient:     "goland",
			clientInstall: true,
			expectError:   "invalid MCP client: goland.",
		},
		{
			name:          "windsurf client",
			ideClient:     "windsurf",
			clientInstall: true,
			initialConfig: "",
			expectedConfig: `{	
				"mcpServers": {
				  "defang": {
					"command": "npx",
					"args": [
					  "-y",
					  "defang@latest",
					  "mcp",
					  "serve"
					]
				  }
				}
			  }`,
		},
		{
			name:          "windsurf client not installed",
			ideClient:     "windsurf",
			clientInstall: false,
			initialConfig: "",
			expectedConfig: `{
				"mcpServers": {
				  "defang": {
					"command": "npx",
					"args": [
					  "-y",
					  "defang@latest",
					  "mcp",
					  "serve"
					]
				  }
				}
			  }`,
			expectError: "The client windsurf you are trying to setup is not install or not found in your system path, please try again after installing.",
		},
		{
			name:          "cursor client",
			ideClient:     "cursor",
			clientInstall: true,
			initialConfig: "",
			expectedConfig: `{
				"mcpServers": {
				  "defang": {
					"command": "npx",
					"args": [
					  "-y",
					  "defang@latest",
					  "mcp",
					  "serve"
					]
				  }
				}
			  }`,
		},
		{
			name:          "cursor client not installed",
			ideClient:     "cursor",
			clientInstall: false,
			initialConfig: "",
			expectedConfig: `{
				"mcpServers": {
				  "defang": {
					"command": "npx",
					"args": [
					  "-y",
					  "defang@latest",
					  "mcp",
					  "serve"
					]
				  }
				}
			  }`,
			expectError: "The client cursor you are trying to setup is not install or not found in your system path, please try again after installing.",
		},
		{
			name:          "Vscode config with other mcp servers",
			ideClient:     "vscode",
			clientInstall: true,
			initialConfig: `{
				"git.blame.editorDecoration.enabled": true,
				"gitlens.ai.model": "vscode",
				"gitlens.ai.vscode.model": "copilot:gpt-4o",
				"go.languageServerFlags": [],
				"mcp": {
				  "servers": {
					"defang": {
					  "args": [
						"mcp",
						"serve"
					  ],
					  "command": "defang",
					  "type": "stdio"
					},
					"git": {
					  "args": [
						"mcp-server-git"
					  ],
					  "command": "uvx"
					}
				  }
				},
				"security.workspace.trust.untrustedFiles": "open"
			  }`,
			expectedConfig: `{
				"git.blame.editorDecoration.enabled": true,
				"gitlens.ai.model": "vscode",
				"gitlens.ai.vscode.model": "copilot:gpt-4o",
				"go.languageServerFlags": [],
				"mcp": {
				  "servers": {
					"defang": {
					  "args": [
						"-y",
						"defang@latest",
						"mcp",
						"serve"
					  ],
					  "command": "npx",
					  "type": "stdio"
					},
					"git": {
					  "args": [
						"mcp-server-git"
					  ],
					  "command": "uvx"
					}
				  }
				},
				"security.workspace.trust.untrustedFiles": "open"
			  }`,
		},
		{
			name:          "Vscode insiders config with only defang mcp server",
			ideClient:     "insiders",
			clientInstall: true,
			initialConfig: "",
			expectedConfig: `{
				"mcp": {
				"servers": {
					"defang": {
					"args": [
						"-y",
						"defang@latest",
						"mcp",
						"serve"
					],
					"command": "npx",
					"type": "stdio"
					}
				}
				}
			}`,
		},
		{
			name:          "Fresh install of claude config",
			ideClient:     "claude",
			clientInstall: true,
			initialConfig: "",
			expectedConfig: `{
			"mcpServers": {
				"defang": {
				"command": "npx",
				"args": [
					"-y",
					"defang@latest",
					"mcp",
					"serve"
				]
				}
			}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			client := tt.ideClient
			orgGetClientConfigPath := getClientConfigPath

			// Mock getClientConfigPath by overriding for this test, there is a
			getClientConfigPath = func(client string) (string, error) {
				var configPath string
				switch strings.ToLower(client) {
				case "windsurf", "codeium":
					configPath = filepath.Join(tempDir, ".codeium", "windsurf", "mcp_config.json")
				case "claude":
					configPath = filepath.Join(tempDir, ".config", "Claude", "claude_desktop_config.json")
				case "cursor":
					configPath = filepath.Join(tempDir, ".cursor", "mcp.json")
				case "vscode", "code":
					configPath = filepath.Join(tempDir, "Library", "Application Support", "Code", "User", "settings.json")
				case "vscode-insiders", "insiders":
					configPath = filepath.Join(tempDir, "Library", "Application Support", "Code - Insiders", "User", "settings.json")
				default:
					return "", fmt.Errorf("unsupported client: %s", client)
				}

				return configPath, nil
			}

			configPath, _ := getClientConfigPath(client)
			// If client install is needed we need to mock the install path to the temp directory
			if tt.clientInstall {
				configDir := filepath.Dir(configPath)
				if err := os.MkdirAll(configDir, 0755); err != nil {
					t.Fatalf("failed to create config directory: %v", err)
				}
			}

			fmt.Println("Config path:", configPath)

			if tt.initialConfig != "" {
				// Write the initial config to the config file
				err := os.WriteFile(configPath, []byte(tt.initialConfig), 0644)
				if err != nil {
					t.Fatalf("failed to write initial config file: %v", err)
				}
			}

			// Call SetupClient
			err := SetupClient(client)
			if err != nil && tt.expectError == "" {
				t.Fatalf("unexpected error: %v", err)
			}

			if err != nil && tt.expectError != "" {
				if strings.Contains(err.Error(), tt.expectError) {
					// We expect this error, so we can continue
					return
				}

				t.Fatalf("expected error: %v, got: %v", tt.expectError, err)
			}

			// Check if the config file was written correctly
			if tt.expectedConfig != "" {
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("failed to read config file: %v", err)
				}

				if slices.Contains(ValidVSCodeClients, client) {
					var config VScodeConfig
					var expectedConfig VScodeConfig

					err = json.Unmarshal([]byte(tt.expectedConfig), &expectedConfig)

					if err != nil {
						t.Fatalf("failed to unmarshal expected config: %v", err)
					}

					err = json.Unmarshal(data, &config)
					if err != nil {
						t.Fatalf("failed to unmarshal config file: %v", err)
					}

					if !reflect.DeepEqual(config, expectedConfig) {
						t.Fatalf("expected config: %v, got: %v", expectedConfig, config)
					}
				} else {
					var config MCPConfig
					var expectedConfig MCPConfig

					err = json.Unmarshal([]byte(tt.expectedConfig), &expectedConfig)
					if err != nil {
						t.Fatalf("failed to unmarshal expected config: %v", err)
					}
					err = json.Unmarshal(data, &config)
					if err != nil {
						t.Fatalf("failed to unmarshal config file: %v", err)
					}

					if !reflect.DeepEqual(config, expectedConfig) {
						t.Fatalf("expected config: %v, got: %v", expectedConfig, config)
					}
				}
			}

			t.Cleanup(func() {
				os.Remove(configPath)
				getClientConfigPath = orgGetClientConfigPath
			})
		})
	}
}
