package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/mcp/logger"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:         "mcp",
	Short:       "Manage MCP Server for defang",
	Annotations: authNeededAnnotation,
}

var mcpServerCmd = &cobra.Command{
	Use:         "serve",
	Short:       "Start defang MCP server",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize the terminal with our custom writers
		logger.InitTerminal()

		// Initialize logger
		logger.InitLogger()

		defer logger.Logger.Sync()

		// Setup knowledge base
		if err := mcp.SetupKnowledgeBase(); err != nil {
			logger.Sugar.Errorw("Failed to setup knowledge base", "error", err)
			return err
		}

		logger.Sugar.Info("Starting Defang MCP server")

		// Create a new MCP server
		logger.Sugar.Info("Creating MCP server")
		s := server.NewMCPServer(
			"Defang Services",
			"1.0.0",
			server.WithResourceCapabilities(true, true), // Enable resource management and notifications
			server.WithPromptCapabilities(true),         // Enable interactive prompts
			server.WithToolCapabilities(true),           // Enable dynamic tool list updates
			server.WithLogging(),                        // Enable detailed logging
			server.WithInstructions("You are an MCP server for Defang Services. Your role is to manage and deploy services efficiently using the provided tools and resources."),
		)

		logger.Sugar.Info("MCP server created successfully")

		// Setup resources
		resources.SetupResources(s)

		// Setup tools
		tools.SetupTools(s)

		// Start the server
		logger.Sugar.Info("Starting Defang Services MCP server")
		if err := server.ServeStdio(s); err != nil {
			logger.Sugar.Errorw("Server error", "error", err)
			return err
		}

		logger.Sugar.Info("Server shutdown")

		return nil
	},
}

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

// ValidClients is a list of supported MCP clients
var ValidClients = []string{
	"claude",
	"windsurf",
	"cursor",
	"vscode",
}

// IsValidClient checks if the provided client is in the list of valid clients
func IsValidClient(client string) bool {
	for _, validClient := range ValidClients {
		if validClient == client {
			return true
		}
	}
	return false
}

// GetValidClientsString returns a formatted string of valid clients
func GetValidClientsString() string {
	return strings.Join(ValidClients, ", ")
}

// getClientConfigPath returns the path to the config file for the given client
func getClientConfigPath(client string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	var configPath string
	switch client {
	case "windsurf", "cascade", "codeium":
		configPath = filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json")
	case "claude":
		if runtime.GOOS == "darwin" {
			configPath = filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		} else if runtime.GOOS == "windows" {
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
	case "vscode":
		if runtime.GOOS == "darwin" {
			configPath = filepath.Join(homeDir, "Library", "Application Support", "Code", "User", "settings.json")
		} else if runtime.GOOS == "windows" {
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
	default:
		return "", fmt.Errorf("unsupported client: %s", client)
	}

	return configPath, nil
}

// getDefangMCPConfig returns the default MCP config for Defang
func getDefangMCPConfig() MCPServerConfig {
	return MCPServerConfig{
		Command: "defang",
		Args:    []string{"-C", "/Users/defang/bin", "mcp", "serve"},
	}
}

// getVSCodeDefangMCPConfig returns the default MCP config for Defang in VSCode format
func getVSCodeDefangMCPConfig() VSCodeMCPServerConfig {
	return VSCodeMCPServerConfig{
		Type:    "stdio",
		Command: "defang",
		Args:    []string{"-C", "/Users/defang/bin", "mcp", "serve"},
	}
}

// getVSCodeServerConfig returns a map with the VSCode-specific MCP server config
func getVSCodeServerConfig() map[string]interface{} {
	config := getVSCodeDefangMCPConfig()
	return map[string]interface{}{
		"type":    config.Type,
		"command": config.Command,
		"args":    config.Args,
	}
}

// handleVSCodeConfig handles the special case for VSCode settings.json
func handleVSCodeConfig(configPath string) error {
	// Create or update the config file
	var existingData map[string]interface{}

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
					"defang": getVSCodeServerConfig(),
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
					"defang": getVSCodeServerConfig(),
				}
			} else {
				serversMap, ok := serversData.(map[string]interface{})
				if !ok {
					serversMap = make(map[string]interface{})
				}

				// Add or update the Defang MCP server config
				serversMap["defang"] = getVSCodeServerConfig()

				mcpMap["servers"] = serversMap
			}

			existingData["mcp"] = mcpMap
		}
	} else {
		// File doesn't exist, create a new config with minimal settings
		existingData = map[string]interface{}{
			"mcp": map[string]interface{}{
				"servers": map[string]interface{}{
					"defang": getVSCodeServerConfig(),
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

var mcpSetupCmd = &cobra.Command{
	Use:         "setup",
	Short:       "Setup MCP client for defang mcp server",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := cmd.Flags().GetString("client")
		// Validate client
		if !IsValidClient(client) {
			return fmt.Errorf("invalid MCP client: %s. Valid MCP clients are: %s", client, GetValidClientsString())
		}

		fmt.Printf("Setting up MCP client for: %s\n", client)

		// Get the config path for the client
		configPath, err := getClientConfigPath(client)
		if err != nil {
			return err
		}

		fmt.Printf("Config path: %s\n", configPath)

		// Create the directory if it doesn't exist
		configDir := filepath.Dir(configPath)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		// Handle VSCode settings.json specially
		if client == "vscode" {
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

			// Add or update the Defang MCP server config
			config.MCPServers["defang"] = getDefangMCPConfig()

			// Write the config to the file
			data, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			if err := os.WriteFile(configPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}
		}

		fmt.Printf("Successfully configured %s to use Defang MCP server\n", client)

		// Prompt to restart the client if it's running
		if err := PromptForRestart(client); err != nil {
			fmt.Printf("Warning: Failed to restart client: %v\n", err)
		}

		return nil
	},
}
