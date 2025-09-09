package command

import (
	"fmt"
	"os"
	"path/filepath"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/mcp/prompts"
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
		term.SetDebug(true)

		term.Debug("Creating log file")
		logFile, err := os.OpenFile(filepath.Join(cliClient.StateDir, "defang-mcp.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			term.Warnf("Failed to open log file: %v", err)
		} else {
			defer logFile.Close()
			term.DefaultTerm = term.NewTerm(os.Stdin, logFile, logFile)
		}

		// Setup knowledge base
		term.Debug("Setting up knowledge base")
		if err := mcp.SetupKnowledgeBase(); err != nil {
			return fmt.Errorf("failed to setup knowledge base: %w", err)
		}

		// Create a new MCP server
		term.Debug("Creating MCP server")
		s := server.NewMCPServer(
			"Deploy with Defang",
			RootCmd.Version,
			server.WithResourceCapabilities(true, true), // Enable resource management and notifications
			server.WithPromptCapabilities(true),         // Enable interactive prompts
			server.WithToolCapabilities(true),           // Enable dynamic tool list updates
			server.WithInstructions(`
Defang provides tools for deploying web applications to cloud providers (AWS, GCP, Digital Ocean) using a compose.yaml file.

There are a number of available tools to help with deployment, configuration, and manage applications deployed with Defang.

deploy - This tool deploys a web application to the cloud using the compose.yaml file in the application's working directory.
destroy - This tool spins down and removes a deployed project from the cloud, cleaning up all associated resources.
estimate - This tool estimates the cost of running a deployed application based on its resource usage and cloud provider pricing.
services - This tool lists all running services for a deployed application, providing status and resource usage information
list_configs - This tool lists all configuration variables for a deployed application, allowing you to view current settings.
remove_config - This tool removes a configuration variable for a deployed application, allowing you to clean up unused settings.
set_config - This tool sets or updates configuration variables for a deployed application, allowing you to manage environment variables and secrets.
			`),
		)

		cluster := getOrgCluster()

		// Setup resources
		term.Debug("Setting up resources")
		resources.SetupResources(s)

		//setup prompts
		term.Debug("Setting up prompts")
		prompts.SetupPrompts(s, cluster, &providerID)

		// Setup tools
		term.Debug("Setting up tools")
		tools.SetupTools(s, cluster, authPort, &providerID)

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
		term.Debugf("MCP Client: %q", client)
		if err := mcp.SetupClient(client); err != nil {
			return err
		}

		return nil
	},
}
