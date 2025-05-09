package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/auth"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		//set global nonInteractive to false
		nonInteractive = false
		return nil
	},
}

var mcpServerCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start defang MCP server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		authPort, err := cmd.Flags().GetInt("auth-server")
		if err != nil {
			return err
		}

		fmt.Println("auth port: ", authPort)

		logFile, err := os.OpenFile(filepath.Join(cliClient.StateDir, "defang-mcp.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			term.Error("Failed to open log file", "error", err)
			return err
		}
		defer logFile.Close()

		// TODO: Should we still write to a file or we can go back to stderr
		term.DefaultTerm = term.NewTerm(os.Stdin, logFile, logFile)

		// Setup knowledge base
		if err := mcp.SetupKnowledgeBase(); err != nil {
			term.Error("Failed to setup knowledge base", "error", err)
			return err
		}

		term.Info("Starting Defang MCP server")

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
		resources.SetupResources(s)

		// Setup tools
		tools.SetupTools(s, getCluster())

		if authPort != 0 {
			term.Info("Starting Auth Server for Docker login flow")

			serverURL := "http://127.0.0.1:" + strconv.Itoa(authPort)

			var openAuthClient = auth.NewClient("defang-cli", pkg.Getenv("DEFANG_ISSUER", "https://auth.defang.io"))
			// Since we know what out local serverURL is because it pass down by the flag we can get the authorize url early and set it
			ar, _ := openAuthClient.Authorize(serverURL, auth.CodeResponseType, auth.WithPkce(), auth.WithProvider("github"))

			//code channel
			ch := make(chan string)
			go func() {
				select {
				case code := <-ch:
					if err := cli.ExchangeCodeAndSave(cmd.Context(), auth.NewAuthCodeFlow(ar, code), getCluster()); err != nil {
						term.Error("Failed to exchange auth code for access token", "error", err)
					}
					term.Info("Received auth code: ", code)
				}
			}()
			auth.StartAuthCodeFlowWithDocker(cmd.Context(), authPort, &ch, ar)
		}

		// Start the server
		term.Info("Starting Defang Services MCP server")
		term.Println("Starting Defang MCP server")
		if err := server.ServeStdio(s); err != nil {
			return err
		}

		term.Info("Server shutdown")

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
