package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/pelletier/go-toml/v2"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
)

// MCPServerConfig represents the configuration for an MCP server
type MCPServerConfig struct {
	Command string            `json:"command,omitempty" toml:"command,omitempty"`
	Args    []string          `json:"args,omitempty" toml:"args,omitempty"`
	Type    string            `json:"type,omitempty" toml:"type,omitempty"`
	URL     string            `json:"url,omitempty" toml:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty" toml:"env,omitempty"`
	EnvFile string            `json:"envFile,omitempty" toml:"envFile,omitempty"`
	Headers map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

// MCPClient represents the supported MCP clients as an enum
type MCPClient string

const (
	MCPClientClaudeCode       MCPClient = "claude-code"
	MCPClientClaudeDesktop    MCPClient = "claude-desktop"
	MCPClientCodex            MCPClient = "codex"
	MCPClientCursor           MCPClient = "cursor"
	MCPClientKiro             MCPClient = "kiro"
	MCPClientRovo             MCPClient = "rovo"
	MCPClientUnspecified      MCPClient = ""
	MCPClientVSCode           MCPClient = "vscode"
	MCPClientVSCodeCodespaces MCPClient = "vscode-codespaces"
	MCPClientVSCodeInsiders   MCPClient = "vscode-insiders"
	MCPClientWindsurf         MCPClient = "windsurf"
)

// ValidVSCodeClients is a list of supported VSCode MCP clients with shorthand names
var ValidVSCodeClients = []MCPClient{
	MCPClientVSCode,
	MCPClientVSCodeCodespaces,
	MCPClientVSCodeInsiders,
}

// ValidClients is a list of supported MCP clients
var ValidClients = append(
	[]MCPClient{
		MCPClientClaudeCode,
		MCPClientClaudeDesktop,
		MCPClientCodex,
		MCPClientCursor,
		MCPClientKiro,
		MCPClientRovo,
		MCPClientWindsurf,
	},
	ValidVSCodeClients...,
)

// ParseMCPClient parses and validates the MCP client string
func ParseMCPClient(clientStr string) (MCPClient, error) {
	clientStr = strings.ToLower(clientStr)
	client := MCPClient(clientStr)
	if !slices.Contains(ValidClients, client) {
		return "", fmt.Errorf("invalid MCP client: %q. Valid MCP clients are %v", clientStr, ValidClients)
	}
	return client, nil
}

// ValidClientStrings converts ValidClients to []string for survey options
func ValidClientStrings() []string {
	strings := make([]string, len(ValidClients))
	for i, client := range ValidClients {
		strings[i] = string(client)
	}
	return strings
}

// SelectMCPclients prompts the user to select one or more MCP clients
func SelectMCPclients() ([]string, error) {
	var clients []string
	err := survey.AskOne(&survey.MultiSelect{
		Message: "Choose a client(s):",
		Options: ValidClientStrings(),
	}, &clients)
	if err != nil {
		return nil, fmt.Errorf("failed to select MCP client(s): %w", err)
	}

	return clients, nil
}

// ClientInfo defines where each client stores its MCP configuration
type ClientInfo struct {
	configFile string // Configuration file name
	useHomeDir bool   // True if config goes directly in home dir, false if in system config dir
}

var claudeCodeConfig = ClientInfo{
	configFile: ".claude.json",
	useHomeDir: true,
}

var claudeDesktopConfig = ClientInfo{
	configFile: "Claude/claude_desktop_config.json",
	useHomeDir: false,
}

var codexConfig = ClientInfo{
	configFile: ".codex/config.toml",
	useHomeDir: true,
}

var cursorConfig = ClientInfo{
	configFile: ".cursor/mcp.json",
	useHomeDir: true,
}

var kiroConfig = ClientInfo{
	configFile: ".kiro/settings/mcp.json",
	useHomeDir: true,
}

var rovoConfig = ClientInfo{
	configFile: ".rovodev/mcp.json",
	useHomeDir: true,
}

var vscodeConfig = ClientInfo{
	configFile: "Code/User/mcp.json",
	useHomeDir: false,
}

var vscodeCodespacesConfig = ClientInfo{
	configFile: ".vscode-remote/data/User/mcp.json",
	useHomeDir: true,
}

var vscodeInsidersConfig = ClientInfo{
	configFile: "Code - Insiders/User/mcp.json",
	useHomeDir: false,
}

var windsurfConfig = ClientInfo{
	configFile: ".codeium/windsurf/mcp_config.json",
	useHomeDir: true,
}

// clientRegistry maps client names to their configuration details
var clientRegistry = map[MCPClient]ClientInfo{
	MCPClientClaudeCode:       claudeCodeConfig,
	MCPClientClaudeDesktop:    claudeDesktopConfig,
	MCPClientCodex:            codexConfig,
	MCPClientCursor:           cursorConfig,
	MCPClientKiro:             kiroConfig,
	MCPClientRovo:             rovoConfig,
	MCPClientVSCode:           vscodeConfig,
	MCPClientVSCodeCodespaces: vscodeCodespacesConfig,
	MCPClientVSCodeInsiders:   vscodeInsidersConfig,
	MCPClientWindsurf:         windsurfConfig,
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

	if client == MCPClientCodex {
		if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
			return filepath.Join(codexHome, "config.toml"), nil
		}
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

func readConfig(configPath string) (map[string]any, error) {
	config := make(map[string]any)
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if strings.TrimSpace(string(configBytes)) == "" {
		return config, nil
	}

	if strings.HasSuffix(configPath, ".toml") {
		err = toml.Unmarshal(configBytes, &config)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
		return config, nil
	}

	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

func extractServerMap(config map[string]any, key string) (map[string]any, error) {
	if config == nil {
		return make(map[string]any), nil
	}
	if _, exists := config[key]; !exists {
		return make(map[string]any), nil
	}
	serverMap, ok := config[key].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid \"%s\" config format", key)
	}
	return serverMap, nil
}

func writeConfigFile(configPath string, data any) error {
	var configBytes []byte
	var err error
	if strings.HasSuffix(configPath, ".toml") {
		configBytes, err = toml.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal config to TOML: %w", err)
		}
	} else {
		configBytes, err = json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config to JSON: %w", err)
		}
	}

	// #nosec G306 - config file does not contain sensitive data
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func configureDefangMCPServer(configPath string, client MCPClient) error {
	config, err := readConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to read existing config file: %w", err)
	}

	var key string
	switch client {
	case MCPClientVSCode, MCPClientVSCodeInsiders, MCPClientVSCodeCodespaces:
		key = "servers"
	case MCPClientCodex:
		// Codex uses TOML format and a different key
		key = "mcp_servers"
	default:
		// Default to JSON format with standard key
		key = "mcpServers"
	}

	serverMap, err := extractServerMap(config, key)
	if err != nil {
		return fmt.Errorf("failed to extract server map: %w", err)
	}

	name := "defang"
	command, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	serverConfig := MCPServerConfig{
		Command: command,
		Args:    []string{"mcp", "serve", "--client", string(client)},
	}

	if client == MCPClientVSCode || client == MCPClientVSCodeInsiders {
		serverConfig.Type = "stdio"
	}

	serverMap[name] = serverConfig
	config[key] = serverMap
	return writeConfigFile(configPath, config)
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

	err = configureDefangMCPServer(configPath, client)
	if err != nil {
		return fmt.Errorf("failed to update MCP config file for client %q: %w", client, err)
	}

	term.Infof("Ensure %s is upgraded to the latest version and restarted for MCP settings to take effect.\n", client)

	return nil
}
