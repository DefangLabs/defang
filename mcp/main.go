package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	host           = "fabric-prod1.defang.dev:443"
	gitHubClientId = "7b41848ca116eac4b125" // Default GitHub OAuth app ID
	logFilePath    = "logs/defang_mcp.log"
)

// Global logger
var logger *zap.Logger

// Sugar logger for more convenient logging
var sugar *zap.SugaredLogger

// Initialize logger
func initLogger() {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		fmt.Printf("Failed to create logs directory: %v\n", err)
		os.Exit(1)
	}

	// Configure zap logger
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout", logFilePath}
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Disable buffering for real-time updates
	config.DisableStacktrace = false
	config.DisableCaller = false
	config.OutputPaths = []string{"stdout", logFilePath}
	config.ErrorOutputPaths = []string{"stderr", logFilePath}

	// Create logger with custom options for real-time logging
	var err error
	logger, err = config.Build(zap.WithCaller(true))
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Create sugar logger
	sugar = logger.Sugar()

	// Set up periodic flushing of logs
	go func() {
		for {
			time.Sleep(100 * time.Millisecond)
			logger.Sync()
		}
	}()

	// Note: We'll call logger.Sync() in the main function
}

func getTokenFile(fabric string) string {
	if host, _, _ := net.SplitHostPort(fabric); host != "" {
		fabric = host
	}
	return filepath.Join(client.StateDir, fabric)
}

func getExistingToken(fabric string) string {
	// First check environment variable
	accessToken := os.Getenv("DEFANG_ACCESS_TOKEN")
	if accessToken != "" {
		return accessToken
	}

	// Then check token file
	tokenFile := getTokenFile(fabric)
	all, err := os.ReadFile(tokenFile)
	if err == nil {
		return string(all)
	}

	return ""
}

func saveToken(fabric, token string) error {
	tokenFile := getTokenFile(fabric)

	sugar.Infow("Saving token to file", "file", tokenFile)

	// Create state directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0700); err != nil {
		sugar.Errorw("Failed to create state directory", "error", err)
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write token to file
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		sugar.Errorw("Failed to save token", "error", err)
		return fmt.Errorf("failed to save token: %w", err)
	}

	sugar.Info("Token saved successfully")
	return nil
}

func validateToken(ctx context.Context, token string) bool {
	if token == "" {
		sugar.Debug("Empty token provided for validation")
		return false
	}

	sugar.Debug("Validating token")

	// Create a temporary client to validate token
	tempClient := client.NewGrpcClient(host, token, types.TenantName(""))

	// Try to get user info
	_, err := tempClient.WhoAmI(ctx)
	if err != nil {
		sugar.Debugw("Token validation failed", "error", err)
		return false
	}

	sugar.Debug("Token validated successfully")
	return true
}

func login(ctx context.Context, grpcClient client.GrpcClient) (string, error) {
	sugar.Info("Starting GitHub authentication flow")

	// Start GitHub auth flow
	code, err := github.StartAuthCodeFlow(ctx, gitHubClientId)
	if err != nil {
		sugar.Errorw("Failed to start auth flow", "error", err)
		return "", fmt.Errorf("failed to start auth flow: %w", err)
	}

	sugar.Info("Successfully obtained GitHub auth code, exchanging for token")

	// Exchange code for token with unrestricted access
	resp, err := grpcClient.Token(ctx, &defangv1.TokenRequest{
		AuthCode:  code,
		Tenant:    string(types.DEFAULT_TENANT),
		Scope:     nil, // nil scope = unrestricted access
		ExpiresIn: uint32(24 * time.Hour.Seconds()),
	})
	if err != nil {
		sugar.Errorw("Failed to exchange code for token", "error", err)
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}

	sugar.Info("Successfully obtained access token")
	return resp.AccessToken, nil
}

func getValidToken(ctx context.Context) (string, error) {
	sugar.Info("Getting valid token")

	// Try to get existing token
	token := getExistingToken(host)

	// Validate token if we have one
	if token != "" {
		sugar.Debug("Found existing token, validating")
		if validateToken(ctx, token) {
			sugar.Info("Using existing valid token")
			return token, nil
		}
		sugar.Info("Existing token is invalid, getting new token")
	} else {
		sugar.Info("No existing token found, getting new token")
	}

	// Create a temporary gRPC client for login
	tempClient := client.NewGrpcClient(host, "", types.TenantName(""))

	// Get token through GitHub auth flow
	token, err := login(ctx, tempClient)
	if err != nil {
		sugar.Errorw("Failed to login", "error", err)
		return "", fmt.Errorf("failed to login: %w", err)
	}

	// Save token to file and environment
	if err := saveToken(host, token); err != nil {
		sugar.Warnw("Failed to save token", "error", err)
	}
	os.Setenv("DEFANG_ACCESS_TOKEN", token)
	sugar.Info("Token saved to environment variable")

	return token, nil
}

func getServices(ctx context.Context, token string, composePath string) (*defangv1.GetServicesResponse, error) {
	sugar.Info("Getting services")
	sugar.Infow("Using compose path", "path", composePath)

	// Check if the path exists
	fileInfo, err := os.Stat(composePath)
	if err != nil {
		sugar.Errorw("Failed to access compose path", "error", err, "path", composePath)
		return nil, fmt.Errorf("failed to access compose path %s: %w", composePath, err)
	}

	var configPaths []string
	if fileInfo.IsDir() {
		// If it's a directory, look for compose.yaml in that directory
		composeFilePath := filepath.Join(composePath, "compose.yaml")
		if _, err := os.Stat(composeFilePath); err != nil {
			sugar.Errorw("No compose.yaml found in directory", "error", err, "dir", composePath)
			return nil, fmt.Errorf("no compose.yaml found in directory %s: %w", composePath, err)
		}
		configPaths = []string{composeFilePath}
		sugar.Infow("Found compose.yaml in directory", "path", composeFilePath)
	} else {
		// If it's a file, use it directly
		configPaths = []string{composePath}
	}

	// Create a loader with explicit config paths
	loader := compose.NewLoader(compose.WithPath(configPaths...))

	// Get project name from loader
	projectName, err := loader.LoadProjectName(ctx)
	if err != nil {
		sugar.Errorw("Failed to load project name", "error", err)
		return nil, fmt.Errorf("failed to load project name: %w", err)
	}

	sugar.Infow("Project name loaded", "project", projectName)

	// Create a gRPC client with token
	grpcClient := client.NewGrpcClient(host, token, types.TenantName(""))

	// Create a provider using cli.NewProvider
	provider, err := cli.NewProvider(ctx, client.ProviderDefang, grpcClient)
	if err != nil {
		sugar.Errorw("Failed to create provider", "error", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	sugar.Infow("Getting services for project", "project", projectName)

	// First try to get services for the specific project
	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		sugar.Warnw("Failed to get services for specific project, trying to list all services", "error", err, "project", projectName)
		
		// If getting services for the specific project fails, try to get all services
		allServicesResponse, allErr := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: ""})
		if allErr != nil {
			sugar.Errorw("Failed to get all services", "error", allErr)
			return nil, fmt.Errorf("failed to get services for project %s: %w and failed to get all services: %v", projectName, err, allErr)
		}
		
		sugar.Infow("Retrieved all services", "count", len(allServicesResponse.Services))
		return allServicesResponse, nil
	}

	sugar.Infow("Successfully retrieved services for project", "project", projectName, "count", len(servicesResponse.Services))
	return servicesResponse, nil
}

func main() {
	// Initialize logger
	initLogger()
	defer logger.Sync()

	sugar.Info("Starting Defang MCP server")

	// Parse flags
	projectDir := flag.String("C", ".", "Project directory containing compose.yaml")
	flag.Parse()

	sugar.Infow("Command line flags parsed", "projectDir", *projectDir)

	// Set up context
	ctx := context.Background()

	// Change to project directory
	sugar.Infow("Changing to project directory", "dir", *projectDir)
	if err := os.Chdir(*projectDir); err != nil {
		sugar.Errorw("Failed to change directory", "error", err, "dir", *projectDir)
		os.Exit(1)
	}

	// Get valid token
	sugar.Info("Getting authentication token")
	token, err := getValidToken(ctx)
	if err != nil {
		sugar.Errorw("Failed to get valid token", "error", err)
		os.Exit(1)
	}

	// Create a new MCP server
	sugar.Info("Creating MCP server")
	s := server.NewMCPServer(
		"Defang Services",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)
	sugar.Info("MCP server created successfully")

	// Create a tool for listing services
	sugar.Info("Creating services tool")
	servicesTool := mcp.NewTool("services",
		mcp.WithDescription("List information about services in Defang"),
		mcp.WithString("compose",
			mcp.Required(),
			mcp.Description("The server should check in the current working directory if not ask the user to provide a path to the directory containing the compose.yaml file"),
		),
	)
	sugar.Debug("Services tool created")

	// Add the services tool handler - make it non-blocking
	sugar.Info("Adding services tool handler")
	s.AddTool(servicesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get services
		sugar.Info("Services tool called - fetching services from Defang")

		composePath, ok := request.Params.Arguments["compose"].(string)
		if !ok {
			sugar.Errorw("Compose parameter is missing", "error", errors.New("compose parameter is missing"))
		}

		// zap log as well
		sugar.Info("Compose directory path: ", composePath)
		fmt.Println("Compose directory path:", composePath)

		// Create a timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Use a goroutine to prevent blocking
		resultChan := make(chan *defangv1.GetServicesResponse, 1)
		errChan := make(chan error, 1)

		go func() {
			sugar.Debug("Starting goroutine to fetch services")
			// Get services
			resp, err := getServices(timeoutCtx, token, composePath)
			if err != nil {
				errChan <- err
				return
			}
			resultChan <- resp
		}()

		// Wait for result with a timeout
		select {
		case services := <-resultChan:
			sugar.Infow("Successfully retrieved services", "count", len(services.Services))

			// Format output
			var output strings.Builder
			fmt.Fprintf(&output, "Number of services: %d\n\n", len(services.Services))

			for _, service := range services.Services {
				fmt.Fprintf(&output, "Service: %s\n", service.Service.Name)
				fmt.Fprintf(&output, "Deployment: %s\n", service.Etag)
				fmt.Fprintf(&output, "Public FQDN: %s\n", service.PublicFqdn)
				fmt.Fprintf(&output, "Private FQDN: %s\n", service.PrivateFqdn)
				fmt.Fprintf(&output, "Status: %s\n\n", service.Status)
			}

			return mcp.NewToolResultText(output.String()), nil
		case err := <-errChan:
			sugar.Errorw("Failed to get services in tool handler", "error", err)
			return mcp.NewToolResultText(fmt.Sprintf("Failed to get services: %v", err)), nil
		case <-timeoutCtx.Done():
			sugar.Warn("Services operation timed out")
			return mcp.NewToolResultText("Operation timed out. Please try again later."), nil
		}
	})

	// Start the server
	sugar.Info("Starting Defang Services MCP server")
	if err := server.ServeStdio(s); err != nil {
		sugar.Errorw("Server error", "error", err)
		os.Exit(1)
	}
	sugar.Info("Server shutdown")
}
