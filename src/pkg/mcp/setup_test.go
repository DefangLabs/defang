package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
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
		codexHome     string
		expectedPath  string
		expectedError bool
	}{
		// Windsurf/Cascade/Codeium tests
		{
			name:         "windsurf",
			client:       MCPClientWindsurf,
			expectedPath: filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"),
		},

		// Claude Desktop tests - Darwin
		{
			name:         "claude_desktop_darwin",
			client:       MCPClientClaudeDesktop,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
		},

		// Claude Desktop tests - Windows with APPDATA
		{
			name:         "claude_desktop_windows_with_appdata",
			client:       MCPClientClaudeDesktop,
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join("C:\\Users\\TestUser\\AppData\\Roaming", "Claude", "claude_desktop_config.json"),
		},

		// Claude Desktop tests - Windows without APPDATA
		{
			name:         "claude_desktop_windows_without_appdata",
			client:       MCPClientClaudeDesktop,
			goos:         "windows",
			appData:      "",
			expectedPath: filepath.Join(homeDir, "AppData", "Roaming", "Claude", "claude_desktop_config.json"),
		},

		// Claude Desktop tests - Linux with XDG_CONFIG_HOME
		{
			name:          "claude_desktop_linux_with_xdg",
			client:        MCPClientClaudeDesktop,
			goos:          "linux",
			xdgConfigHome: "/home/testuser/.config",
			expectedPath:  filepath.Join("/home/testuser/.config", "Claude", "claude_desktop_config.json"),
		},

		// Claude Desktop tests - Linux without XDG_CONFIG_HOME
		{
			name:          "claude_desktop_linux_without_xdg",
			client:        MCPClientClaudeDesktop,
			goos:          "linux",
			xdgConfigHome: "",
			expectedPath:  filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json"),
		},

		// Claude Code tests - Darwin
		{
			name:         "claude_code_darwin",
			client:       MCPClientClaudeCode,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, ".claude.json"),
		},

		// Claude Code tests - Linux with XDG_CONFIG_HOME
		{
			name:          "claude_code_linux_with_xdg",
			client:        MCPClientClaudeCode,
			goos:          "linux",
			xdgConfigHome: "/home/testuser",
			expectedPath:  filepath.Join(homeDir, ".claude.json"),
		},

		// Claude Code tests - Linux without XDG_CONFIG_HOME
		{
			name:          "claude_code_linux_without_xdg",
			client:        MCPClientClaudeCode,
			goos:          "linux",
			xdgConfigHome: "",
			expectedPath:  filepath.Join(homeDir, ".claude.json"),
		},

		// Claude Code tests - Windows with APPDATA
		{
			name:         "claude_code_windows_with_appdata",
			client:       MCPClientClaudeCode,
			goos:         "windows",
			appData:      "C:\\Users\\TestUser\\AppData\\Roaming",
			expectedPath: filepath.Join(homeDir, ".claude.json"),
		},

		// Claude code tests - Windows without APPDATA
		{
			name:         "claude_code_windows_without_appdata",
			client:       MCPClientClaudeCode,
			goos:         "windows",
			appData:      "",
			expectedPath: filepath.Join(homeDir, ".claude.json"),
		},

		// Cursor tests
		{
			name:         "cursor",
			client:       MCPClientCursor,
			expectedPath: filepath.Join(homeDir, ".cursor", "mcp.json"),
		},

		// Kiro tests - Darwin
		{
			name:         "kiro_darwin",
			client:       MCPClientKiro,
			goos:         "darwin",
			expectedPath: filepath.Join(homeDir, ".kiro", "settings", "mcp.json"),
		},

		// Kiro tests - Linux
		{
			name:         "kiro_linux",
			client:       MCPClientKiro,
			goos:         "linux",
			expectedPath: filepath.Join(homeDir, ".kiro", "settings", "mcp.json"),
		},

		// Codex tests - default path
		{
			name:         "codex_default",
			client:       MCPClientCodex,
			expectedPath: filepath.Join(homeDir, ".codex", "config.toml"),
		},

		// Codex tests - CODEX_HOME override
		{
			name:         "codex_with_env",
			client:       MCPClientCodex,
			codexHome:    filepath.Join(homeDir, "custom-codex-home"),
			expectedPath: filepath.Join(homeDir, "custom-codex-home", "config.toml"),
		},

		// VSCode tests - Darwin
		{
			name:         "vscode_darwin",
			client:       MCPClientVSCode,
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

			t.Setenv("CODEX_HOME", tt.codexHome)

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

func TestWriteConfig(t *testing.T) {
	// This test function will use handleVSCodeConfig and handleStandardConfig to make sure that is not overwritten existing data
	// and only add or append our defangmcp config, or if there not an existing file; make one and write it.
	test := []struct {
		name          string
		fileExists    bool
		vscodeConfig  bool
		existingData  string
		expectedData  string
		expectedError bool
	}{
		{
			name:         "vscode_new_file",
			fileExists:   false,
			vscodeConfig: true,
			existingData: "",
			expectedData: `{
  "servers": {
    "defang": {
      "args": [
        "mcp",
        "serve",
        "--client",
        "vscode"
      ],
      "command": %s,
      "type": "stdio"
    }
  }
}`,
		},
		{
			name:         "vscode_other_mcp_server_without_defang",
			fileExists:   true,
			vscodeConfig: true,
			existingData: `{
	"servers": {
		"notion": {
			"command": "npx",
			"args": [
				"-y",
				"@notionhq/notion-mcp-server"
			],
			"env": {
				"OPENAPI_MCP_HEADERS": {
					"Authorization": "Bearer ${input:NOTION_TOKEN}",
					"Notion-Version": "2022-06-28"
				}
			},
			"type": "stdio"
		},
		"github": {
			"url": "https://api.githubcopilot.com/mcp/"
		}
	},
	"inputs": [
		{
			"id": "NOTION_TOKEN",
			"type": "promptString",
			"description": "Notion API Token (https://www.notion.so/profile/integrations)",
			"password": true
		}
	]
}`,
			expectedData: `{
  "inputs": [
    {
      "description": "Notion API Token (https://www.notion.so/profile/integrations)",
      "id": "NOTION_TOKEN",
      "password": true,
      "type": "promptString"
    }
  ],
  "servers": {
    "defang": {
      "args": [
        "mcp",
        "serve",
        "--client",
        "vscode"
      ],
      "command": %s,
      "type": "stdio"
    },
    "github": {
      "url": "https://api.githubcopilot.com/mcp/"
    },
    "notion": {
      "args": [
        "-y",
        "@notionhq/notion-mcp-server"
      ],
      "command": "npx",
      "env": {
        "OPENAPI_MCP_HEADERS": {
          "Authorization": "Bearer ${input:NOTION_TOKEN}",
          "Notion-Version": "2022-06-28"
        }
      },
      "type": "stdio"
    }
  }
}`,
		},
		{
			name:         "vscode_other_mcp_server_with_defang",
			fileExists:   true,
			vscodeConfig: true,
			existingData: `{
  "inputs": [
    {
      "description": "Notion API Token (https://www.notion.so/profile/integrations)",
      "id": "NOTION_TOKEN",
      "password": true,
      "type": "promptString"
    }
  ],
  "servers": {
    "defang": {
      "args": [
        "mcp",
        "serve",
        "--client",
        "vscode"
      ],
      "command": "OLD_OUTDATED_DEFANG_LOCATION",
      "type": "stdio"
    },
    "github": {
      "url": "https://api.githubcopilot.com/mcp/"
    },
    "notion": {
      "args": [
        "-y",
        "@notionhq/notion-mcp-server"
      ],
      "command": "npx",
      "env": {
        "OPENAPI_MCP_HEADERS": {
          "Authorization": "Bearer ${input:NOTION_TOKEN}",
          "Notion-Version": "2022-06-28"
        }
      },
      "type": "stdio"
    }
  }
}`,
			expectedData: `{
  "inputs": [
    {
      "description": "Notion API Token (https://www.notion.so/profile/integrations)",
      "id": "NOTION_TOKEN",
      "password": true,
      "type": "promptString"
    }
  ],
  "servers": {
    "defang": {
      "args": [
        "mcp",
        "serve",
        "--client",
        "vscode"
      ],
      "command": %s,
      "type": "stdio"
    },
    "github": {
      "url": "https://api.githubcopilot.com/mcp/"
    },
    "notion": {
      "args": [
        "-y",
        "@notionhq/notion-mcp-server"
      ],
      "command": "npx",
      "env": {
        "OPENAPI_MCP_HEADERS": {
          "Authorization": "Bearer ${input:NOTION_TOKEN}",
          "Notion-Version": "2022-06-28"
        }
      },
      "type": "stdio"
    }
  }
}`,
		},
		{
			name:          "vscode_invalid_json_file",
			fileExists:    true,
			existingData:  `{invalid json}`,
			expectedError: true,
		},
		{
			name:          "vscode_servers_not_object",
			fileExists:    true,
			vscodeConfig:  true,
			existingData:  `{"servers": "not an object}`,
			expectedError: true,
		},
		{
			name:         "vscode_config_new_file_empty",
			fileExists:   true,
			vscodeConfig: true,
			existingData: "",
			expectedData: `{
  "servers": {
    "defang": {
      "args": [
        "mcp",
        "serve",
        "--client",
        "vscode"
      ],
      "command": %s,
      "type": "stdio"
    }
  }
}`,
		},
		{
			name:         "vscode_config_new_file_with_whitespace",
			fileExists:   true,
			vscodeConfig: true,
			existingData: "   \t  \n ",
			expectedData: `{
  "servers": {
    "defang": {
      "args": [
        "mcp",
        "serve",
        "--client",
        "vscode"
      ],
      "command": %s,
      "type": "stdio"
    }
  }
}`,
		},
		{
			name:         "standard_config_new_file",
			fileExists:   false,
			existingData: "",
			expectedData: `{
  "mcpServers": {
    "defang": {
      "command": %s,
      "args": [
        "mcp",
        "serve",
        "--client",
        "cursor"
      ]
    }
  }
}`,
		},
		{
			name:       "standard_config_other_mcp_server_without_defang",
			fileExists: true,
			existingData: `{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": ""
      }
    },
    "stripe": {
      "command": "npx",
      "args": [
        "-y",
        "@stripe/mcp",
        "--tools=all",
        "--api-key="
      ]
    }
  }
}`,
			expectedData: `{
  "mcpServers": {
    "defang": {
      "command": %s,
      "args": [
        "mcp",
        "serve",
        "--client",
        "cursor"
      ]
    },
    "github": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": ""
      }
    },
    "stripe": {
      "command": "npx",
      "args": [
        "-y",
        "@stripe/mcp",
        "--tools=all",
        "--api-key="
      ]
    }
  }
}`,
		},
		{
			name:       "standard_config_other_mcp_server_with_defang",
			fileExists: true,
			existingData: `{
  "mcpServers": {
    "defang": {
      "command": "OLD_OUTDATED_DEFANG_LOCATION",
      "args": [
        "mcp",
        "serve",
        "--client",
        "cursor"
      ]
    },
    "github": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": ""
      }
    },
    "stripe": {
      "command": "npx",
      "args": [
        "-y",
        "@stripe/mcp",
        "--tools=all",
        "--api-key="
      ]
    }
  }
}`,
			expectedData: `{
  "mcpServers": {
    "defang": {
      "command": %s,
      "args": [
        "mcp",
        "serve",
        "--client",
        "cursor"
      ]
    },
    "github": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-github"
      ],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": ""
      }
    },
    "stripe": {
      "command": "npx",
      "args": [
        "-y",
        "@stripe/mcp",
        "--tools=all",
        "--api-key="
      ]
    }
  }
}`,
		},
		{
			name:          "standard_config_invalid_json_file",
			fileExists:    true,
			existingData:  `{invalid json}`,
			expectedError: true,
		},
		{
			name:          "standard_config_not_object",
			fileExists:    true,
			existingData:  `{"mcpServers": "not an object}`,
			expectedError: true,
		},
		{
			name:         "standard_config_new_file_empty",
			fileExists:   true,
			existingData: "",
			expectedData: `{
  "mcpServers": {
    "defang": {
      "command": %s,
      "args": [
        "mcp",
        "serve",
        "--client",
        "cursor"
      ]
    }
  }
}`,
		},
		{
			name:         "standard_config_new_file_with_whitespace",
			fileExists:   true,
			existingData: "   \t  \n ",
			expectedData: `{
  "mcpServers": {
    "defang": {
      "command": %s,
      "args": [
        "mcp",
        "serve",
        "--client",
        "cursor"
      ]
    }
  }
}`,
		},
	}
	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			tempFilePath := filepath.Join(tempDir, "mcp.json")
			t.Cleanup(func() {
				_ = os.Remove(tempFilePath)
			})

			// Get the actual executable path that handleVSCodeConfig and handleStandardConfig will use
			executablePath, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}

			if tt.fileExists {
				if err := os.WriteFile(tempFilePath, []byte(tt.existingData), 0644); err != nil {
					t.Fatal(err)
				}
			}

			var client MCPClient
			if tt.vscodeConfig {
				client = MCPClientVSCode
			} else {
				client = MCPClientCursor
			}
			err = configureDefangMCPServer(tempFilePath, client)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error for %s but got none", client)
				}
				return // Don't continue with file comparison if we expected an error
			} else {
				if err != nil {
					t.Fatalf("Unexpected error for %s: %v", client, err)
				}
			}

			if err != nil {
				t.Fatalf("Unexpected error for %s: %v", client, err)
			}

			expectedData := fmt.Sprintf(tt.expectedData, fmt.Sprintf(`"%s"`, executablePath))

			actualContent, err := os.ReadFile(tempFilePath)
			if err != nil {
				t.Fatal(err)
			}

			if len(bytes.Clone(actualContent)) == 0 {
				actualContent = []byte(`{}`)
			}

			var actualJSON, expectedJSON map[string]interface{}
			if err := json.Unmarshal(actualContent, &actualJSON); err != nil {
				t.Fatalf("Failed to unmarshal actual content: %v\nContent: %s", err, string(actualContent))
			}
			if err := json.Unmarshal([]byte(expectedData), &expectedJSON); err != nil {
				t.Fatalf("Failed to unmarshal expected data: %v\nData: %s", err, expectedData)
			}
			if !reflect.DeepEqual(actualJSON, expectedJSON) {
				t.Errorf("JSON output does not match expected.\nActual: %v\nExpected: %v", actualJSON, expectedJSON)
			}
		})
	}
}

func TestHandleCodexConfig(t *testing.T) {
	executablePath, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to get executable: %v", err)
	}

	tests := []struct {
		name                string
		fileExists          bool
		existingData        string
		otherServers        map[string]MCPServerConfig
		expectProfileActive string
		expectedError       bool
	}{
		{
			name: "codex_new_file",
		},
		{
			name:       "codex_existing_without_defang",
			fileExists: true,
			existingData: `
[profile]
active = "default"

[mcp_servers.docs]
command = "npx"
args = ["-y", "docs-server"]
`,
			otherServers: map[string]MCPServerConfig{
				"docs": {
					Command: "npx",
					Args:    []string{"-y", "docs-server"},
				},
			},
			expectProfileActive: "default",
		},
		{
			name:       "codex_existing_with_defang",
			fileExists: true,
			existingData: `
[mcp_servers.defang]
command = "OLD_PATH"
args = ["mcp", "serve", "--client", "codex"]

[mcp_servers.aux]
command = "npx"
args = ["-y", "aux-server"]
`,
			otherServers: map[string]MCPServerConfig{
				"aux": {
					Command: "npx",
					Args:    []string{"-y", "aux-server"},
				},
			},
		},
		{
			name:          "codex_invalid_toml",
			fileExists:    true,
			existingData:  `invalid = "toml"\nunterminated`,
			expectedError: true,
		},
		{
			name:          "codex_mcp_servers_not_table",
			fileExists:    true,
			existingData:  `mcp_servers = ["bad"]`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.toml")

			if tt.fileExists {
				if err := os.WriteFile(configPath, []byte(tt.existingData), 0644); err != nil {
					t.Fatalf("failed to write existing config: %v", err)
				}
			}

			err := configureDefangMCPServer(configPath, MCPClientCodex)
			if tt.expectedError {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			content, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("failed to read config: %v", err)
			}

			var data map[string]any
			if err := toml.Unmarshal(content, &data); err != nil {
				t.Fatalf("failed to unmarshal resulting config: %v", err)
			}

			mcpServersValue, ok := data["mcp_servers"]
			if !ok {
				t.Fatalf("expected mcp_servers section to be present")
			}

			mcpServers, ok := mcpServersValue.(map[string]any)
			if !ok {
				t.Fatalf("expected mcp_servers to be a map, got %T", mcpServersValue)
			}

			defangRaw, ok := mcpServers["defang"]
			if !ok {
				t.Fatalf("expected defang server to be present")
			}

			defang, ok := defangRaw.(map[string]any)
			if !ok {
				t.Fatalf("expected defang server to be a map, got %T", defangRaw)
			}

			commandValue, ok := defang["command"].(string)
			if !ok {
				t.Fatalf("expected defang command to be string, got %T", defang["command"])
			}

			if commandValue != executablePath {
				t.Fatalf("expected defang command to be %q, got %q", executablePath, commandValue)
			}

			argsValue, ok := defang["args"]
			if !ok {
				t.Fatal("expected defang args to be present")
			}

			args := toStringSlice(t, argsValue)
			expectedArgs := []string{"mcp", "serve", "--client", "codex"}
			if len(args) != len(expectedArgs) {
				t.Fatalf("expected defang args %v, got %v", expectedArgs, args)
			}
			for i, arg := range expectedArgs {
				if args[i] != arg {
					t.Fatalf("expected defang args %v, got %v", expectedArgs, args)
				}
			}

			for serverName, expected := range tt.otherServers {
				serverValue, ok := mcpServers[serverName]
				if !ok {
					t.Fatalf("expected server %q to be present", serverName)
				}

				serverMap, ok := serverValue.(map[string]any)
				if !ok {
					t.Fatalf("expected server %q to be a map, got %T", serverName, serverValue)
				}

				cmd, ok := serverMap["command"].(string)
				if !ok {
					t.Fatalf("expected command for server %q to be string, got %T", serverName, serverMap["command"])
				}

				if cmd != expected.Command {
					t.Fatalf("expected command for server %q to be %q, got %q", serverName, expected.Command, cmd)
				}

				actualArgs := toStringSlice(t, serverMap["args"])
				if len(actualArgs) != len(expected.Args) {
					t.Fatalf("expected args %v for server %q, got %v", expected.Args, serverName, actualArgs)
				}
				for i, arg := range expected.Args {
					if actualArgs[i] != arg {
						t.Fatalf("expected args %v for server %q, got %v", expected.Args, serverName, actualArgs)
					}
				}
			}

			if tt.expectProfileActive != "" {
				profileValue, ok := data["profile"].(map[string]any)
				if !ok {
					t.Fatalf("expected profile section to be a map, got %T", data["profile"])
				}

				active, ok := profileValue["active"].(string)
				if !ok {
					t.Fatalf("expected profile.active to be string, got %T", profileValue["active"])
				}

				if active != tt.expectProfileActive {
					t.Fatalf("expected profile.active %q, got %q", tt.expectProfileActive, active)
				}
			}
		})
	}
}

func toStringSlice(t *testing.T, value any) []string {
	t.Helper()
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		result := make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				t.Fatalf("expected string elements, got %T", item)
			}
			result[i] = str
		}
		return result
	default:
		t.Fatalf("expected slice for args, got %T", value)
	}
	return nil
}
