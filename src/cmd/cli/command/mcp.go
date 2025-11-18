package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP Server for defang",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		//set global nonInteractive to false
		config.NonInteractive = false
	},
}

var mcpServerCmd = &cobra.Command{
	Use:     "serve",
	Aliases: []string{"server"},
	Short:   "Start defang MCP server",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ideClient, _ := cmd.Flags().GetString("client")

		mcpClient, err := mcp.ParseMCPClient(ideClient)
		if err != nil {
			term.Warnf("Unable to parse MCP client: %v", err)
			mcpClient = mcp.MCPClientUnspecified
		}

		term.Debug("Creating log file")
		logFile, err := os.OpenFile(filepath.Join(cliClient.StateDir, "defang-mcp.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			term.Warnf("Failed to open log file: %v", err)
		} else {
			defer logFile.Close()
			term.DefaultTerm = term.NewTerm(os.Stdin, logFile, logFile)
			term.SetDebug(true)
		}

		// Create a new MCP server
		term.Debug("Creating MCP server")
		s, err := mcp.NewDefangMCPServer(RootCmd.Version, getCluster(), &config.ProviderID, mcpClient, tools.DefaultToolCLI{})
		if err != nil {
			return fmt.Errorf("failed to create MCP server: %w", err)
		}

		// Start the server
		term.Println("Starting Defang MCP server")
		if err := server.ServeStdio(s); err != nil {
			return err
		}

		term.Println("Server shutdown")

		return nil
	},
}

var mcpSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup MCP client for defang mcp server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		term.Debug("Setting up MCP client")
		client, _ := cmd.Flags().GetString("client")

		if client != "" {
			// Aliases mapping
			switch client {
			case "code":
				client = string(mcp.MCPClientVSCode)
			case "code-insiders":
				client = string(mcp.MCPClientVSCodeInsiders)
			case "cascade", "codeium":
				client = string(mcp.MCPClientWindsurf)
			}

			term.Debugf("Using MCP client flag: %q", client)
			if err := mcp.SetupClient(client); err != nil {
				return err
			}
		} else {
			term.Debugf("Using MCP client picker: %q", client)
			clients, err := mcp.SelectMCPclients()
			if err != nil {
				return err
			}
			for _, client := range clients {
				term.Debugf("Selected MCP client using picker: %q", client)

				if err := mcp.SetupClient(client); err != nil {
					return err
				}
			}
		}

		return nil
	},
}
