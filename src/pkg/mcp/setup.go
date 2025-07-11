package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
)

// MCPServerConfig represents the configuration for an MCP server
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	EnvFile string            `json:"envFile,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPConfig represents the configuration file structure
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// VSCodeConfig represents the VSCode settings.json structure
type VSCodeConfig struct {
	MCP struct {
		Servers map[string]VSCodeMCPServerConfig `json:"servers"`
	} `json:"mcp"`
	// Other VSCode settings can be preserved with this field
	Other map[string]interface{} `json:"-"`
}

// VSCodeMCPServerConfig represents the configuration for a VSCode MCP server
type VSCodeMCPServerConfig struct {
	Type    string            `json:"type"`          // Required: "stdio" or "sse"
	Command string            `json:"command"`       // Required for stdio
	Args    []string          `json:"args"`          // Required for stdio
	URL     string            `json:"url,omitempty"` // Required for sse
	Env     map[string]string `json:"env,omitempty"`
	EnvFile string            `json:"envFile,omitempty"`
	Headers map[string]string `json:"headers,omitempty"` // For sse
}

// ValidVSCodeClients is a list of supported VSCode MCP clients with shorthand names
var ValidVSCodeClients = []string{
	"vscode",
	"code",
	"vscode-insiders",
	"insiders",
}

// ValidClients is a list of supported MCP clients
var ValidClients = append(
	[]string{
		"claude",
		"windsurf",
		"cursor",
	},
	ValidVSCodeClients...,
)

// isValidClient checks if the provided client is in the list of valid clients
func isValidClient(client string) bool {
	return slices.Contains(ValidClients, client)
}

// getClientConfigPath returns the path to the config file for the given client
func getClientConfigPath(goos string, client string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	var configPath string
	switch strings.ToLower(client) {
	case "windsurf", "cascade", "codeium":
		configPath = filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json")
	case "claude":
		if goos == "darwin" {
			configPath = filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		} else if goos == "windows" {
			appData := os.Getenv("APPDATA")
			if appData == "" {
				appData = filepath.Join(homeDir, "AppData", "Roaming")
			}
			configPath = filepath.Join(appData, "Claude", "claude_desktop_config.json")
		} else {
			configHome := os.Getenv("XDG_CONFIG_HOME")
			if configHome == "" {
				configHome = filepath.Join(homeDir, ".config")
			}
			configPath = filepath.Join(configHome, "Claude", "claude_desktop_config.json")
		}
	case "cursor":
		configPath = filepath.Join(homeDir, ".cursor", "mcp.json")
	case "vscode", "code":
		if goos == "darwin" {
			configPath = filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json")
		} else if goos == "windows" {
			appData := os.Getenv("APPDATA")
			if appData == "" {
				appData = filepath.Join(homeDir, "AppData", "Roaming")
			}
			configPath = filepath.Join(appData, "Code", "User", "settings.json")
		} else {
			configHome := os.Getenv("XDG_CONFIG_HOME")
			if configHome == "" {
				configHome = filepath.Join(homeDir, ".config")
			}
			configPath = filepath.Join(configHome, "Code/User/settings.json")
		}
	case "vscode-insiders", "insiders":
		if goos == "darwin" {
			configPath = filepath.Join(homeDir, "Library", "Application Support", "Code - Insiders", "User", "settings.json")
		} else if goos == "windows" {
			appData := os.Getenv("APPDATA")
			if appData == "" {
				appData = filepath.Join(homeDir, "AppData", "Roaming")
			}
			configPath = filepath.Join(appData, "Code - Insiders", "User", "settings.json")
		} else {
			configHome := os.Getenv("XDG_CONFIG_HOME")
			if configHome == "" {
				configHome = filepath.Join(homeDir, ".config")
			}
			configPath = filepath.Join(configHome, "Code - Insiders/User/settings.json")
		}
	default:
		return "", fmt.Errorf("unsupported client: %s", client)
	}

	return configPath, nil
}

// getDefangMCPConfig returns the default MCP config for Defang
func getDefangMCPConfig() (*MCPServerConfig, error) {
	currentPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	return &MCPServerConfig{
		Command: currentPath,
		Args:    []string{"mcp", "serve"},
	}, nil
}

// getVSCodeDefangMCPConfig returns the default MCP config for Defang in VSCode format
func getVSCodeDefangMCPConfig() (*VSCodeMCPServerConfig, error) {
	currentPath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &VSCodeMCPServerConfig{
		Type:    "stdio",
		Command: currentPath,
		Args:    []string{"mcp", "serve"},
	}, nil
}

// getVSCodeServerConfig returns a map with the VSCode-specific MCP server config
func getVSCodeServerConfig() (map[string]interface{}, error) {
	config, err := getVSCodeDefangMCPConfig()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"type":    config.Type,
		"command": config.Command,
		"args":    config.Args,
	}, nil
}

// handleVSCodeConfig handles the special case for VSCode settings.json
func handleVSCodeConfig(configPath string) error {
	// Create or update the config file
	var existingData map[string]interface{}
	config, err := getVSCodeServerConfig()
	if err != nil {
		return fmt.Errorf("failed to get VSCode MCP config: %w", err)
	}

	// Check if the file exists
	if _, err := os.Stat(configPath); err == nil {
		// File exists, read it
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		// Parse the JSON into a generic map to preserve all settings
		if err := json.Unmarshal(data, &existingData); err != nil {
			// If we can't parse it, start fresh
			existingData = make(map[string]interface{})
		}

		// Check if mcp section exists
		mcpData, ok := existingData["mcp"]
		if !ok {
			// Create new mcp section
			existingData["mcp"] = map[string]interface{}{
				"servers": map[string]interface{}{
					"defang": config,
				},
			}
		} else {
			// Update existing mcp section
			mcpMap, ok := mcpData.(map[string]interface{})
			if !ok {
				mcpMap = make(map[string]interface{})
			}

			serversData, ok := mcpMap["servers"]
			if !ok {
				mcpMap["servers"] = map[string]interface{}{
					"defang": config,
				}
			} else {
				serversMap, ok := serversData.(map[string]interface{})
				if !ok {
					serversMap = make(map[string]interface{})
				}

				// Add or update the Defang MCP server config
				serversMap["defang"] = config

				mcpMap["servers"] = serversMap
			}

			existingData["mcp"] = mcpMap
		}
	} else {
		// File doesn't exist, create a new config with minimal settings
		existingData = map[string]interface{}{
			"mcp": map[string]interface{}{
				"servers": map[string]interface{}{
					"defang": config,
				},
			},
		}
	}

	// Write the config to the file
	data, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func SetupClient(client string) error {
	// Validate client
	if !isValidClient(client) {
		return fmt.Errorf("invalid MCP client: %q. Valid MCP clients are: %v", client, strings.Join(ValidClients, ", "))
	}

	track.Evt("MCP Setup Client: ", track.P("client", client))

	// Get the config path for the client
	configPath, err := getClientConfigPath(runtime.GOOS, client)
	if err != nil {
		return err
	}

	term.Infof("Updating %q\n", configPath)

	// Create the directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Handle VSCode settings.json specially
	if slices.Contains(ValidVSCodeClients, client) {
		if err := handleVSCodeConfig(configPath); err != nil {
			return err
		}
	} else {
		// For all other clients, use the standard format
		var config MCPConfig

		// Check if the file exists
		if _, err := os.Stat(configPath); err == nil {
			// File exists, read it
			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config file: %w", err)
			}

			// Parse the JSON
			if err := json.Unmarshal(data, &config); err != nil {
				// If we can't parse it, start fresh
				config = MCPConfig{
					MCPServers: make(map[string]MCPServerConfig),
				}
			}
		} else {
			// File doesn't exist, create a new config
			config = MCPConfig{
				MCPServers: make(map[string]MCPServerConfig),
			}
		}

		if config.MCPServers == nil {
			config.MCPServers = make(map[string]MCPServerConfig)
		}

		defangConfig, err := getDefangMCPConfig()
		if err != nil {
			return fmt.Errorf("failed to get Defang MCP config: %w", err)
		}
		// Add or update the Defang MCP server config
		config.MCPServers["defang"] = *defangConfig

		// Write the config to the file
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}
	}

	term.Infof("Restart %s for the changes to take effect.\n", client)

	return nil
}
