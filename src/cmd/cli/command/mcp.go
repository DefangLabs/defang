package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/login"
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
		nonInteractive = false
	},
}

var mcpServerCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start defang MCP server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		authPort, _ := cmd.Flags().GetInt("auth-server")
		term.SetDebug(true)

		term.Debug("Creating log file")
		logFile, err := os.OpenFile(filepath.Join(cliClient.StateDir, "defang-mcp.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			term.Warnf("Failed to open log file: %v", err)
		} else {
			defer logFile.Close()
			term.DefaultTerm = term.NewTerm(os.Stdin, logFile, logFile)
		}

		cluster := getCluster()

		// Create a new MCP server
		term.Debug("Creating MCP server")
		s, err := mcp.NewDefangMCPServer(RootCmd.Version, cluster, authPort, &providerID)
		if err != nil {
			return fmt.Errorf("failed to create MCP server: %w", err)
		}

		// Start auth server for docker login flow
		if authPort != 0 {
			term.Debug("Starting Auth Server for MCP-in-Docker login flow")
			term.Debug("Function invoked: cli.InteractiveLoginInsideDocker")

			go func() {
				if err := login.InteractiveLoginInsideDocker(cmd.Context(), cluster, authPort); err != nil {
					term.Error("Failed to start auth server", "error", err)
				}
			}()
		}

		httpServer := server.NewStreamableHTTPServer(s)

		// Create a channel to listen for OS signals
		sigChan := make(chan os.Signal, 1)

		// Register the channel to receive SIGINT (Ctrl+C) and SIGTERM
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		term.Println("Application started. Press Ctrl+C to trigger graceful shutdown.")

		// Goroutine to handle signals
		go func() {
			sig := <-sigChan // Blocks until a signal is received
			term.Printf("\nReceived signal: %v. Initiating graceful shutdown...\n", sig)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			httpServer.Shutdown(shutdownCtx)

			os.Exit(0) // Exit the application after cleanup
		}()

		// Start the server
		term.Println("Starting Defang MCP server on :63546")
		if err := httpServer.Start(":63546"); err != nil {
			term.Error(err)
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
