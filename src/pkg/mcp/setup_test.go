package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
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
			expectedPath: filepath.Join(homeDir, ".cursor", "settings.json"),
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
        "serve"
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
        "serve"
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
        "serve"
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
        "serve"
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
			name:         "standard_config_new_file",
			fileExists:   false,
			existingData: "",
			expectedData: `{
  "mcpServers": {
    "defang": {
      "command": %s,
      "args": [
        "mcp",
        "serve"
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
        "serve"
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
        "serve"
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
        "serve"
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

			var typeOfConfig string

			if tt.vscodeConfig {
				typeOfConfig = "vscode mcp config"
				err = handleVSCodeConfig(tempFilePath)
			} else {
				typeOfConfig = "standard mcp config"
				err = handleStandardConfig(tempFilePath)
			}

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error for %s but got none", typeOfConfig)
				}
				return // Don't continue with file comparison if we expected an error
			} else {
				if err != nil {
					t.Fatalf("Unexpected error for %s: %v", typeOfConfig, err)
				}
			}

			if err != nil {
				t.Fatalf("Unexpected error for %s: %v", typeOfConfig, err)
			}

			expectedData := fmt.Sprintf(tt.expectedData, fmt.Sprintf(`"%s"`, executablePath))

			actualContent, err := os.ReadFile(tempFilePath)
			if err != nil {
				t.Fatal(err)
			}

			if err := pkg.Diff(string(actualContent), expectedData); err != nil {
				t.Error(err)
			}
		})
	}
}
