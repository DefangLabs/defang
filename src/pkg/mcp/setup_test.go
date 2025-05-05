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

func TestSetupClient(t *testing.T) {
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
			name:          "windsurf client fresh install",
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
			expectError: "the client windsurf you are trying to setup is not install or not found in your system path. Please try again after installing.",
		},
		{
			name:          "cursor client fresh install",
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
			expectError: "the client cursor you are trying to setup is not install or not found in your system path. Please try again after installing.",
		},
		{
			name:          "Vscode pre-existing config with other mcp servers",
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
			name:          "Vscode pre-existing config with no mcp servers",
			ideClient:     "vscode",
			clientInstall: true,
			initialConfig: `{
				"git.blame.editorDecoration.enabled": true,
				"gitlens.ai.model": "vscode",
				"gitlens.ai.vscode.model": "copilot:gpt-4o",
				"go.languageServerFlags": [],
				"mcp": {
					"servers": {
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
						}
					}
				},
				"security.workspace.trust.untrustedFiles": "open"
			}`,
		},
		{
			name:          "Vscode insider pre-existing config with 1 pre-existing mcp server",
			ideClient:     "insiders",
			clientInstall: true,
			initialConfig: `{
				"git.blame.editorDecoration.enabled": true,
				"gitlens.ai.model": "vscode",
				"gitlens.ai.vscode.model": "copilot:gpt-4o",
				"go.languageServerFlags": [],
				"mcp": {
				  "servers": {
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
			name:          "Vscode insiders no config",
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
		{
			name:          "claude config with empty mcpServers field",
			ideClient:     "claude",
			clientInstall: true,
			initialConfig: `{
				"mcpServers": {}
				}`,
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
			name:          "cursor pre-existing config with 1 mcp server",
			ideClient:     "cursor",
			clientInstall: true,
			initialConfig: `{
				"mcpServers": {
					"git": {
					"command": "python3",
					"args": [
					"-m",
					"mcp_server_git"
					]
					}
				}
				}`,
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
					},
					"git": {
					"command": "python3",
					"args": [
					"-m",
					"mcp_server_git"
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

			// Mock getClientConfigPath by overriding for this test, there is a separate already covering this function
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

			// If there is an initial config, write it to the config file
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

			// If we expect an error, check if it matches the expected error
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

func TestGetClientConfigPath(t *testing.T) {
	// Save original OS and restore it after tests

	tests := []struct {
		name       string
		clientName string
		os         string
		expected   string
	}{
		{
			name:       "windsurf on darwin",
			clientName: "windsurf",
			os:         "darwin",
			expected:   filepath.Join(".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:       "windsurf on linux",
			clientName: "windsurf",
			os:         "linux",
			expected:   filepath.Join(".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:       "windsurf on windows",
			clientName: "windsurf",
			os:         "windows",
			expected:   filepath.Join(".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:       "codeium on darwin",
			clientName: "codeium",
			os:         "darwin",
			expected:   filepath.Join(".codeium", "windsurf", "mcp_config.json"),
		},
		{
			name:       "vscode on darwin",
			clientName: "vscode",
			os:         "darwin",
			expected:   filepath.Join("Library", "Application Support", "Code", "User", "settings.json"),
		},
		{
			name:       "vscode on linux",
			clientName: "vscode",
			os:         "linux",
			expected:   filepath.Join(".config", "Code", "User", "settings.json"),
		},
		{
			name:       "vscode on windows",
			clientName: "vscode",
			os:         "windows",
			expected:   filepath.Join("AppData", "Roaming", "Code", "User", "settings.json"),
		},
		{
			name:       "vscode-insiders on darwin",
			clientName: "vscode-insiders",
			os:         "darwin",
			expected:   filepath.Join("Library", "Application Support", "Code - Insiders", "User", "settings.json"),
		},
		{
			name:       "vscode-insiders on linux",
			clientName: "vscode-insiders",
			os:         "linux",
			expected:   filepath.Join(".config", "Code - Insiders", "User", "settings.json"),
		},
		{
			name:       "vscode-insiders on windows",
			clientName: "vscode-insiders",
			os:         "windows",
			expected:   filepath.Join("AppData", "Roaming", "Code - Insiders", "User", "settings.json"),
		},
		{
			name:       "claude on darwin",
			clientName: "claude",
			os:         "darwin",
			expected:   filepath.Join("Library", "Application Support", "Claude", "claude_desktop_config.json"),
		},
		{
			name:       "claude on linux",
			clientName: "claude",
			os:         "linux",
			expected:   filepath.Join(".config", "Claude", "claude_desktop_config.json"),
		},
		{
			name:       "claude on windows",
			clientName: "claude",
			os:         "windows",
			expected:   filepath.Join("AppData", "Roaming", "Claude", "claude_desktop_config.json"),
		},
		{
			name:       "cursor on darwin",
			clientName: "cursor",
			os:         "darwin",
			expected:   filepath.Join(".cursor", "mcp.json"),
		},
		{
			name:       "cursor on linux",
			clientName: "cursor",
			os:         "linux",
			expected:   filepath.Join(".cursor", "mcp.json"),
		},
		{
			name:       "cursor on windows",
			clientName: "cursor",
			os:         "windows",
			expected:   filepath.Join(".cursor", "mcp.json"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set mock OS for this test case
			originalOS := currentOS
			currentOS = test.os
			fmt.Println("Current OS:", currentOS)

			homeDir, err := os.UserHomeDir()
			if err != nil {
				t.Fatalf("failed to get home directory: %v", err)
			}

			fmt.Println("Home directory test:", homeDir)
			t.Log("Home directory test:", homeDir)

			result, err := getClientConfigPath(test.clientName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != filepath.Join(homeDir, test.expected) {
				t.Fatalf("expected %s, got %s", filepath.Join(homeDir, test.expected), result)
			}

			t.Cleanup(func() {
				currentOS = originalOS
			})
		})
	}
}
