package mcp

import (
	"encoding/json"
	"errors"
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

// VSCodeConfig represents the VSCode mcp.json structure
type VSCodeConfig struct {
	Servers map[string]VSCodeMCPServerConfig `json:"servers"`
	// Other VSCode settings can be preserved with this field
	Other map[string]any `json:"-"`
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

// MCPClient represents the supported MCP clients as an enum
type MCPClient string

const (
	MCPClientVSCode         MCPClient = "vscode"
	MCPClientCode           MCPClient = "code"
	MCPClientVSCodeInsiders MCPClient = "vscode-insiders"
	MCPClientInsiders       MCPClient = "code-insiders"
	MCPClientClaudeDesktop  MCPClient = "claude-desktop"
	MCPClientClaudeCode     MCPClient = "claude-code"
	MCPClientWindsurf       MCPClient = "windsurf"
	MCPClientCascade        MCPClient = "cascade"
	MCPClientCodeium        MCPClient = "codeium"
	MCPClientCursor         MCPClient = "cursor"
	MCPClientKiro           MCPClient = "kiro"
)

// ValidVSCodeClients is a list of supported VSCode MCP clients with shorthand names
var ValidVSCodeClients = []MCPClient{
	MCPClientVSCode,
	MCPClientCode,
	MCPClientVSCodeInsiders,
	MCPClientInsiders,
}

// ValidClients is a list of supported MCP clients
var ValidClients = append(
	[]MCPClient{
		MCPClientClaudeDesktop,
		MCPClientClaudeCode,
		MCPClientWindsurf,
		MCPClientCascade,
		MCPClientCodeium,
		MCPClientCursor,
		MCPClientKiro,
	},
	ValidVSCodeClients...,
)

func ParseMCPClient(clientStr string) (MCPClient, error) {
	clientStr = strings.ToLower(clientStr)
	client := MCPClient(clientStr)
	if !slices.Contains(ValidClients, client) {
		return "", fmt.Errorf("invalid MCP client: %q. Valid MCP clients are %v", clientStr, ValidClients)
	}
	return client, nil
}

// ClientInfo defines where each client stores its MCP configuration
type ClientInfo struct {
	configFile string // Configuration file name
	useHomeDir bool   // True if config goes directly in home dir, false if in system config dir
}

var windsurfConfig = ClientInfo{
	configFile: ".codeium/windsurf/mcp_config.json",
	useHomeDir: true,
}

var vscodeConfig = ClientInfo{
	configFile: "Code/User/mcp.json",
	useHomeDir: false,
}

var codeInsidersConfig = ClientInfo{
	configFile: "Code - Insiders/User/mcp.json",
	useHomeDir: false,
}

var claudeDesktopConfig = ClientInfo{
	configFile: "Claude/claude_desktop_config.json",
	useHomeDir: false,
}

var claudeCodeConfig = ClientInfo{
	configFile: ".claude.json",
	useHomeDir: true,
}

var cursorConfig = ClientInfo{
	configFile: ".cursor/settings.json",
	useHomeDir: true,
}

var kiroConfig = ClientInfo{
	configFile: ".kiro/settings/mcp.json",
	useHomeDir: true,
}

// clientRegistry maps client names to their configuration details
var clientRegistry = map[MCPClient]ClientInfo{
	MCPClientWindsurf:       windsurfConfig,
	MCPClientCascade:        windsurfConfig,
	MCPClientCodeium:        windsurfConfig,
	MCPClientVSCode:         vscodeConfig,
	MCPClientCode:           vscodeConfig,
	MCPClientVSCodeInsiders: codeInsidersConfig,
	MCPClientInsiders:       codeInsidersConfig,
	MCPClientClaudeDesktop:  claudeDesktopConfig,
	MCPClientClaudeCode:     claudeCodeConfig,
	MCPClientCursor:         cursorConfig,
	MCPClientKiro:           kiroConfig,
}

// getSystemConfigDir returns the system configuration directory for the given OS
func getSystemConfigDir(homeDir, goos string) string {
	switch goos {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support")
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return appData
		}
		return filepath.Join(homeDir, "AppData", "Roaming")
	case "linux":
		if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
			return configHome
		}
		return filepath.Join(homeDir, ".config")
	default:
		// Default to Linux behavior
		if configHome := os.Getenv("XDG_CONFIG_HOME"); configHome != "" {
			return configHome
		}
		return filepath.Join(homeDir, ".config")
	}
}

// getClientConfigPath returns the path to the config file for the given client
func getClientConfigPath(homeDir, goos string, client MCPClient) (string, error) {
	clientInfo, exists := clientRegistry[client]
	if !exists {
		return "", fmt.Errorf("unsupported client: %s", client)
	}

	var basePath string
	if clientInfo.useHomeDir {
		// Config goes directly in home directory
		basePath = homeDir
	} else {
		// Config goes in system-specific config directory
		basePath = getSystemConfigDir(homeDir, goos)
	}

	return filepath.Join(basePath, clientInfo.configFile), nil
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
func getVSCodeServerConfig() (map[string]any, error) {
	config, err := getVSCodeDefangMCPConfig()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":    config.Type,
		"command": config.Command,
		"args":    config.Args,
	}, nil
}

// handleVSCodeConfig handles the special case for VSCode mcp.json
func handleVSCodeConfig(configPath string) error {
	// Create or update the config file
	var existingData map[string]any
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
			return fmt.Errorf("failed to unmarshal existing vscode config %w", err)
		}

		// Check if "servers" section exists
		serversSection, ok := existingData["servers"]
		if !ok {
			// Create new "servers" section
			existingData["servers"] = map[string]any{}
			serversSection = existingData["servers"]
		}

		if mcpMap, ok := serversSection.(map[string]any); ok {
			mcpMap["defang"] = config
			existingData["servers"] = mcpMap
		} else {
			return errors.New("failed to assert 'servers' section as map[string]any")
		}
	} else {
		// File doesn't exist, create a new config with minimal settings
		existingData = map[string]any{
			"servers": map[string]any{
				"defang": config,
			},
		}
	}

	// Write the config to the file
	data, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// #nosec G306 - config file does not contain sensitive data
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func handleStandardConfig(configPath string) error {
	// For all other clients, use the standard format
	var existingData map[string]any
	var config MCPConfig

	// Check if the file exists
	if _, err := os.Stat(configPath); err == nil {
		// File exists, read it
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		// Parse the JSON into a generic map to preserve all settings
		if err := json.Unmarshal(data, &existingData); err != nil {
			return fmt.Errorf("failed to unmarshal existing config: %w", err)
		}

		// Try to extract MCPServers from existing data
		if mcpServersData, ok := existingData["mcpServers"]; ok {
			// Convert back to MCPConfig structure
			mcpServersJSON, err := json.Marshal(map[string]any{"mcpServers": mcpServersData})
			if err != nil {
				return fmt.Errorf("failed to marshal mcpServers: %w", err)
			}
			json.Unmarshal(mcpServersJSON, &config)
		}
	} else {
		// File doesn't exist, create a new config
		existingData = make(map[string]any)
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

	// Update the existingData with the new MCPServers
	existingData["mcpServers"] = config.MCPServers

	// Write the config to the file
	data, err := json.MarshalIndent(existingData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// #nosec G306 - config file does not contain sensitive data
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func SetupClient(clientStr string) error {
	client, err := ParseMCPClient(clientStr)
	if err != nil {
		return err
	}

	track.Evt("MCP Setup Client: ", track.P("client", client))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	// Get the config path for the client
	configPath, err := getClientConfigPath(homeDir, runtime.GOOS, client)
	if err != nil {
		return err
	}

	term.Infof("Updating %q\n", configPath)

	// Create the directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Handle VSCode mcp.json specially
	if slices.Contains(ValidVSCodeClients, client) {
		if err := handleVSCodeConfig(configPath); err != nil {
			return err
		}
	} else {
		if err := handleStandardConfig(configPath); err != nil {
			return err
		}
	}

	term.Infof("Restart %s for the changes to take effect.\n", client)

	return nil
}
