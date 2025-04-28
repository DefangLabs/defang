package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mock ValidClients and ValidVSCodeClients if needed
// Otherwise, ensure they are accessible from the test

func TestSetupClient_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		client        string
		initialConfig string // "" for no file, otherwise initial JSON
		expectError   bool
		expectDefang  bool
		expectCommand string
	}{
		{
			name:         "invalid client",
			client:       "code",
			expectError:  true,
			expectDefang: false,
		},
		{
			name:          "new config file",
			client:        "testclient",
			expectError:   false,
			expectDefang:  true,
			expectCommand: "npx",
		},
		{
			name:          "existing config missing MCPServers",
			client:        "testclient2",
			initialConfig: `{"SomeOtherField":123}`,
			expectError:   false,
			expectDefang:  true,
			expectCommand: "npx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			client := tt.client

			getClientConfigPath = func(client string) (string, error) {
				return filepath.Join(tempDir, client+".json"), nil
			}

			configPath, _ := getClientConfigPath(client)

			fmt.Println(configPath)

			// Write initial config if needed
			if tt.initialConfig != "" {
				if err := os.WriteFile(configPath, []byte(tt.initialConfig), 0644); err != nil {
					t.Fatalf("failed to write initial config: %v", err)
				}
			}

			// Call SetupClient
			err := SetupClient(client)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check config file
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("failed to read config file: %v", err)
			}
			var config MCPConfig
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatalf("failed to unmarshal config: %v", err)
			}
			defang, ok := config.MCPServers["defang"]
			if tt.expectDefang && !ok {
				t.Errorf("expected defang server config, not found")
			}
			if tt.expectCommand != "" && defang.Command != tt.expectCommand {
				t.Errorf("expected Command %q, got %q", tt.expectCommand, defang.Command)
			}
		})
	}
}
