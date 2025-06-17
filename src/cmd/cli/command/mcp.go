package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
			"Defang MCP Server",
			RootCmd.Version,
			server.WithResourceCapabilities(true, true), // Enable resource management and notifications
			server.WithPromptCapabilities(true),         // Enable interactive prompts
			server.WithToolCapabilities(true),           // Enable dynamic tool list updates
			server.WithInstructions("Use these tools to deploy projects to the cloud with Defang. These tools also help manage deployed projects, manage config variables, and estimate the monthly cost of deploying a given compose file."),
		)

		// Setup resources
		term.Debug("Setting up resources")
		resources.SetupResources(s)

		// Setup tools
		term.Debug("Setting up tools")
		tools.SetupTools(s, getCluster(), authPort)

		// Start auth server for docker login flow
		if authPort != 0 {
			term.Debug("Starting Auth Server for Docker login flow")
			term.Debug("Function invoked: cli.InteractiveLoginWithDocker")

			go func() {
				if err := cli.InteractiveLoginWithDocker(cmd.Context(), getCluster(), authPort); err != nil {
					term.Error("Failed to start auth server", "error", err)
				}
			}()
		}

		// Start the server
		term.Println("Starting Defang MCP server on :5533")
		mux := http.NewServeMux()
		mux.HandleFunc("/sse", createSSEHandler(s))
		if err := http.ListenAndServe(":5533", mux); err != nil {
			return err
		}

		term.Println("Server shutdown")

		return nil
	},
}

// createSSEHandler creates an HTTP handler for SSE transport of MCP messages
func createSSEHandler(mcpServer *server.MCPServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

		// Handle preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Create a context for this connection
		ctx := r.Context()
		// Read the request body (JSON-RPC message)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			term.Error("Failed to read request body", "error", err)
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		// Process the message through the MCP server
		response := mcpServer.HandleMessage(ctx, json.RawMessage(body))

		// Marshal the response
		responseData, err := json.Marshal(response)
		if err != nil {
			term.Error("Failed to marshal response", "error", err)
			http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			return
		}

		// Send the response as SSE event
		if _, err := fmt.Fprintf(w, "data: %s\n\n", responseData); err != nil {
			term.Error("Failed to write SSE response", "error", err)
			return
		}

		// Flush the response
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

var mcpSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup MCP client for defang mcp server",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		term.Debug("Setting up MCP client")
		client, _ := cmd.Flags().GetString("client")
		client = strings.ToLower(client)
		term.Debug("Client: ", client)
		if err := mcp.SetupClient(client); err != nil {
			return err
		}

		return nil
	},
}
