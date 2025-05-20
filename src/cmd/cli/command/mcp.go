package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/mcp/resources"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
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

		term.Info("Creating log file")
		logFile, err := os.OpenFile(filepath.Join(cliClient.StateDir, "defang-mcp.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			term.Warnf("Failed to open log file: %v", err)
		} else {
			defer logFile.Close()
			term.DefaultTerm = term.NewTerm(os.Stdin, logFile, logFile)
		}

		// Setup knowledge base
		term.Info("Function invoked: mcp.SetupKnowledgeBase")
		if err := mcp.SetupKnowledgeBase(); err != nil {
			return fmt.Errorf("failed to setup knowledge base: %w", err)
		}

		// Create a new MCP server
		term.Info("Creating MCP server")
		s := server.NewMCPServer(
			"Defang Services",
			RootCmd.Version,
			server.WithResourceCapabilities(true, true), // Enable resource management and notifications
			server.WithPromptCapabilities(true),         // Enable interactive prompts
			server.WithToolCapabilities(true),           // Enable dynamic tool list updates
			server.WithInstructions("You are an MCP server for Defang Services. Your role is to manage and deploy services efficiently using the provided tools and resources."),
		)

		// Setup resources
		term.Info("Setting up resources")
		resources.SetupResources(s)

		// Setup tools
		term.Info("Setting up tools")
		tools.SetupTools(s, getCluster(), authPort)

		// Start auth server for docker login flow
		if authPort != 0 {
			term.Info("Starting Auth Server for Docker login flow")
			term.Info("Function invoked: cli.InteractiveLoginWithDocker")

			go func() {
				if err := cli.InteractiveLoginWithDocker(cmd.Context(), getCluster(), authPort); err != nil {
					term.Error("Failed to start auth server", "error", err)
				}
			}()
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
		client, _ := cmd.Flags().GetString("client")
		client = strings.ToLower(client)
		if err := mcp.SetupClient(client); err != nil {
			return err
		}

		return nil
	},
}
