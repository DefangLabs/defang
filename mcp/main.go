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
	logFilePath    = "logs/defang_mcp.json"
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
		server.WithPromptCapabilities(true),
		server.WithLogging(),
	)
	sugar.Info("MCP server created successfully")

	samplePrompt := mcp.NewPrompt("Make dockerfile and compose file",
		mcp.WithPromptDescription("The user should give you a path to a project directory, and you should create a dockerfile and compose file for that project. If there is an app folder, make the dockerfile for that folder. Then make a compose file for original project directory or root of that project directory."),
		mcp.WithArgument("project_path",
			mcp.ArgumentDescription("Path to the project directory"),
			mcp.RequiredArgument(),
		),
	)

	s.AddPrompt(samplePrompt, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		projectPath := request.Params.Arguments["project_path"]
		if projectPath == "" {
			projectPath = "."
			sugar.Warnw("Project path not provided, using current directory", "dir", projectPath)
		}

		return mcp.NewGetPromptResult(
			"Code assistance to make dockerfile and compose file",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(fmt.Sprintf("You are a helpful code writer. I will give you a path which is %s to a project directory, and you should create a dockerfile and compose file for that project. If there is an app folder, make the dockerfile for that folder. Then make a compose file for original project directory or root of that project directory. When creating these files, make sure to use the samples and examples resource for reference of defang. If you need more information, please use the defang documentation resource. When you are creating these files please make sure to scan carefully to expose any ports, start commands, and any other information needed for the project.", projectPath)),
				),
				mcp.NewPromptMessage(
					mcp.RoleAssistant,
					mcp.NewEmbeddedResource(mcp.TextResourceContents{
						MIMEType: "application/json",
						URI:      "doc://knowledge_base.json",
					}),
				),
			},
		), nil
	})

	// Create a documentation resource
	sugar.Info("Creating documentation resource")
	docResource := mcp.NewResource(
		"doc://knowledge_base.json",
		"knowledge_base",
		mcp.WithResourceDescription("Defang documentation for any question or information you need to know about Defang. If you want to look to build dockerfiles and compose files, please use the samples and examples resource"),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(docResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Read the file
		file, err := os.ReadFile("knowledge_base.json")
		if err != nil {
			sugar.Errorw("Failed to read resource file", "error", err, "path", "knowledge_base.json")
			return nil, fmt.Errorf("failed to read resource file knowledge_base.json: %w", err)
		}

		// Return the file content
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				Text:     string(file),
				MIMEType: "application/json",
				URI:      "doc://knowledge_base.json",
			},
		}, nil
	})

	// Create a samples examples resource
	sugar.Info("Creating samples examples resource")
	samplesResource := mcp.NewResource(
		"doc://samples_examples.json",
		"samples_examples",
		mcp.WithResourceDescription("Defang sample projects that should be used for reference when trying to create new dockerfiles and compose files."),
		mcp.WithMIMEType("application/json"),
	)

	// Add samples examples resource
	s.AddResource(samplesResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Read the file
		file, err := os.ReadFile("samples_examples.json")
		if err != nil {
			sugar.Errorw("Failed to read resource file", "error", err, "path", "samples_examples.json")
			return nil, fmt.Errorf("failed to read resource file samples_examples.json: %w", err)
		}

		// Return the file content
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				Text:     string(file),
				MIMEType: "application/json",
				URI:      "doc://samples_examples.json",
			},
		}, nil
	})

	// Create a tool for logging in and getting a new token
	sugar.Info("Creating login tool")
	loginTool := mcp.NewTool("login",
		mcp.WithDescription("Login to Defang"),
	)
	sugar.Debug("Login tool created")

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

	// Create a tool for deployment
	sugar.Info("Creating deployment tool")
	composeUpTool := mcp.NewTool("deploy",
		mcp.WithDescription("Deploy services using defang"),
		mcp.WithString("compose",
			mcp.Required(),
			mcp.Description("Path to the directory containing the compose.yaml file"),
		),
		mcp.WithString("mode",
			mcp.Description("Deployment mode: DEPLOYMENT_MODE_UNSPECIFIED, DEPLOYMENT_MODE_RECREATE, or DEPLOYMENT_MODE_ROLLING_UPDATE"),
		),
		mcp.WithBoolean("wait",
			mcp.Description("Wait for deployment to complete"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds for waiting for deployment to complete"),
		),
	)
	sugar.Debug("Deployment tool created")

	// Create a tool for destroying services
	sugar.Info("Creating destroy tool")
	composeDownTool := mcp.NewTool("destroy",
		mcp.WithDescription("Remove services using defang"),
		mcp.WithString("compose",
			mcp.Required(),
			mcp.Description("Path to the directory containing the compose.yaml file"),
		),
		mcp.WithString("service",
			mcp.Description("Optional service name to remove. If not specified, all services will be removed."),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force removal without confirmation"),
		),
	)
	sugar.Debug("Destroy tool created")

	// Add the login tool handler - make it non-blocking
	sugar.Info("Adding login tool handler")
	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Create a timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Use a goroutine to prevent blocking
		resultChan := make(chan string, 1)
		errChan := make(chan error, 1)

		go func() {
			sugar.Info("Login tool called - initiating login flow")

			sugar.Info("Getting a new token")

			// Create a temporary gRPC client for login
			tempClient := client.NewGrpcClient(host, "", types.TenantName(""))

			// Always get a new token
			token, err := login(timeoutCtx, tempClient)
			if err != nil {
				errChan <- fmt.Errorf("failed to get new token: %w", err)
				return
			}

			// Save the new token
			if err := saveToken(host, token); err != nil {
				sugar.Warnw("Failed to save token", "error", err)
			}
			os.Setenv("DEFANG_ACCESS_TOKEN", token)
			sugar.Info("New token saved to environment variable")

			resultChan <- token
		}()

		// Wait for result or timeout
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("timeout while getting token")
		case err := <-errChan:
			return nil, err
		case token := <-resultChan:
			// Mask the token for security
			maskedToken := "****"
			if len(token) > 4 {
				maskedToken = token[:4] + "****"
			}

			// Format output
			var output strings.Builder
			fmt.Fprintf(&output, "Successfully logged in to Defang\n")
			fmt.Fprintf(&output, "Token preview: %s\n", maskedToken)
			fmt.Fprintf(&output, "Login credentials saved to environment variable DEFANG_ACCESS_TOKEN\n")

			return mcp.NewToolResultText(output.String()), nil
		}
	})

	// Add the deployment tool handler - make it non-blocking
	sugar.Info("Adding deployment tool handler")
	s.AddTool(composeUpTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get compose path
		sugar.Info("Compose up tool called - deploying services")

		composePath, ok := request.Params.Arguments["compose"].(string)
		if !ok {
			sugar.Errorw("Compose parameter is missing", "error", errors.New("compose parameter is missing"))
			return mcp.NewToolResultText("Error: compose parameter is missing"), nil
		}

		// Get optional parameters
		modeStr, _ := request.Params.Arguments["mode"].(string)
		wait, _ := request.Params.Arguments["wait"].(bool)
		timeoutSec, _ := request.Params.Arguments["timeout"].(float64)

		// Default values
		mode := defangv1.DeploymentMode_MODE_UNSPECIFIED
		if modeStr == "DEVELOPMENT" {
			mode = defangv1.DeploymentMode_DEVELOPMENT
		} else if modeStr == "STAGING" {
			mode = defangv1.DeploymentMode_STAGING
		} else if modeStr == "PRODUCTION" {
			mode = defangv1.DeploymentMode_PRODUCTION
		}

		waitTimeout := 5 * time.Minute
		if timeoutSec > 0 {
			waitTimeout = time.Duration(timeoutSec) * time.Second
		}

		// Log parameters
		sugar.Infow("Compose up parameters",
			"composePath", composePath,
			"mode", mode,
			"wait", wait,
			"timeout", waitTimeout)

		// Create a timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute) // Longer timeout for deployment
		defer cancel()

		// Use a goroutine to prevent blocking
		resultChan := make(chan *defangv1.DeployResponse, 1)
		errChan := make(chan error, 1)
		logChan := make(chan string, 100) // Channel for log messages

		go func() {
			sugar.Debug("Starting goroutine to deploy services")

			// Create a progress reporter that sends updates to the log channel
			progressReporter := func(message string) {
				logChan <- message
			}

			// Parse the compose file
			sugar.Info("Parsing compose file")
			progressReporter("Parsing compose file...")

			// Create a loader for the compose file
			loader := compose.NewLoader(compose.WithPath(filepath.Join(composePath, "compose.yaml")))
			project, err := loader.LoadProject(timeoutCtx)
			if err != nil {
				errChan <- fmt.Errorf("failed to parse compose file: %w", err)
				return
			}

			// Get fabric client and provider
			grpcClient := client.NewGrpcClient(host, token, types.TenantName(""))
			provider, err := cli.NewProvider(timeoutCtx, client.ProviderDefang, grpcClient)
			if err != nil {
				errChan <- fmt.Errorf("failed to create provider: %w", err)
				return
			}

			// Deploy the services
			sugar.Info("Deploying services")
			progressReporter(fmt.Sprintf("Deploying services for project %s...", project.Name))

			// Use ComposeUp to deploy the services
			deployResp, _, err := cli.ComposeUp(timeoutCtx, project, grpcClient, provider, compose.UploadModeDigest, mode)
			if err != nil {
				errChan <- fmt.Errorf("failed to deploy services: %w", err)
				return
			}

			// If wait is enabled, wait for deployment to complete
			if wait {
				progressReporter("Waiting for deployment to complete...")

				// Create a context with timeout for waiting
				waitCtx, waitCancel := context.WithTimeout(timeoutCtx, waitTimeout)
				defer waitCancel()

				// Wait for deployment to complete
				err = cli.WaitAndTail(waitCtx, project, grpcClient, provider, deployResp, waitTimeout, time.Now(), true)
				if err != nil {
					if errors.Is(err, context.DeadlineExceeded) {
						progressReporter("Wait timeout exceeded, but deployment is still in progress")
					} else {
						errChan <- fmt.Errorf("error while waiting for deployment: %w", err)
						return
					}
				}
			}

			resultChan <- deployResp
		}()

		// Collect log messages and wait for result with a timeout
		var logs []string
		for {
			select {
			case log := <-logChan:
				logs = append(logs, log)
			case deployResp := <-resultChan:
				sugar.Infow("Successfully deployed services", "etag", deployResp.Etag)

				// Format output
				var output strings.Builder

				// Add collected logs
				for _, log := range logs {
					fmt.Fprintf(&output, "%s\n", log)
				}

				fmt.Fprintf(&output, "\nDeployment successful!\n")
				fmt.Fprintf(&output, "Deployment ID: %s\n\n", deployResp.Etag)

				fmt.Fprintf(&output, "Services:\n")
				for _, serviceInfo := range deployResp.Services {
					fmt.Fprintf(&output, "- %s\n", serviceInfo.Service.Name)
					fmt.Fprintf(&output, "  Public URL: %s\n", serviceInfo.PublicFqdn)
					fmt.Fprintf(&output, "  Status: %s\n", serviceInfo.Status)
				}

				return mcp.NewToolResultText(output.String()), nil
			case err := <-errChan:
				sugar.Errorw("Failed to deploy services", "error", err)

				// Format output with logs and error
				var output strings.Builder

				// Add collected logs
				for _, log := range logs {
					fmt.Fprintf(&output, "%s\n", log)
				}

				fmt.Fprintf(&output, "\nDeployment failed: %v\n", err)
				return mcp.NewToolResultText(output.String()), nil
			case <-timeoutCtx.Done():
				sugar.Warn("Deployment operation timed out")

				// Format output with logs and timeout message
				var output strings.Builder

				// Add collected logs
				for _, log := range logs {
					fmt.Fprintf(&output, "%s\n", log)
				}

				fmt.Fprintf(&output, "\nOperation timed out. The deployment might still be in progress.\n")
				return mcp.NewToolResultText(output.String()), nil
			}
		}
	})

	// Add the destroy tool handler - make it non-blocking
	sugar.Info("Adding destroy tool handler")
	s.AddTool(composeDownTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get compose path
		sugar.Info("Compose down tool called - removing services")

		composePath, ok := request.Params.Arguments["compose"].(string)
		if !ok {
			sugar.Errorw("Compose parameter is missing", "error", errors.New("compose parameter is missing"))
			return mcp.NewToolResultText("Error: compose parameter is missing"), nil
		}

		// Get optional parameters
		serviceName, _ := request.Params.Arguments["service"].(string)
		force, _ := request.Params.Arguments["force"].(bool)

		// Create a slice of service names if a specific service is specified
		var serviceNames []string
		if serviceName != "" {
			serviceNames = []string{serviceName}
		}

		// Log parameters
		sugar.Infow("Compose down parameters",
			"composePath", composePath,
			"serviceName", serviceName,
			"force", force)

		// Create a timeout context
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute) // Timeout for service removal
		defer cancel()

		// Use a goroutine to prevent blocking
		resultChan := make(chan string, 1) // Channel for the etag
		errChan := make(chan error, 1)
		logChan := make(chan string, 100) // Channel for log messages

		go func() {
			sugar.Debug("Starting goroutine to remove services")

			// Create a progress reporter that sends updates to the log channel
			progressReporter := func(message string) {
				logChan <- message
			}

			// Get project name from compose file
			sugar.Info("Loading project from compose file")
			progressReporter("Loading project from compose file...")

			// Create a loader for the compose file
			loader := compose.NewLoader(compose.WithPath(filepath.Join(composePath, "compose.yaml")))
			project, err := loader.LoadProject(timeoutCtx)
			if err != nil {
				errChan <- fmt.Errorf("failed to load project: %w", err)
				return
			}

			projectName := project.Name
			progressReporter(fmt.Sprintf("Project name: %s", projectName))

			// Get fabric client and provider
			grpcClient := client.NewGrpcClient(host, token, types.TenantName(""))
			provider, err := cli.NewProvider(timeoutCtx, client.ProviderDefang, grpcClient)
			if err != nil {
				errChan <- fmt.Errorf("failed to create provider: %w", err)
				return
			}

			// Check if user wants to proceed if force is not enabled
			if !force {
				serviceDesc := "all services"
				if serviceName != "" {
					serviceDesc = "service '" + serviceName + "'"
				}
				progressReporter(fmt.Sprintf("About to remove %s from project %s", serviceDesc, projectName))
				progressReporter("Since 'force' is not enabled, proceeding without confirmation in non-interactive mode.")
			}

			// Remove the services
			sugar.Info("Removing services")
			serviceDesc := "all services"
			if serviceName != "" {
				serviceDesc = "service '" + serviceName + "'"
			}
			progressReporter(fmt.Sprintf("Removing %s from project %s...", serviceDesc, projectName))

			// Use ComposeDown to remove the services
			etag, err := cli.ComposeDown(timeoutCtx, projectName, grpcClient, provider, serviceNames...)
			if err != nil {
				errChan <- fmt.Errorf("failed to remove services: %w", err)
				return
			}

			resultChan <- string(etag)
		}()

		// Collect log messages and wait for result with a timeout
		var logs []string
		for {
			select {
			case log := <-logChan:
				logs = append(logs, log)
			case etag := <-resultChan:
				sugar.Infow("Successfully removed services", "etag", etag)

				// Format output
				var output strings.Builder

				// Add collected logs
				for _, log := range logs {
					fmt.Fprintf(&output, "%s\n", log)
				}

				fmt.Fprintf(&output, "\nServices successfully removed!\n")
				fmt.Fprintf(&output, "Deployment ID: %s\n", etag)

				return mcp.NewToolResultText(output.String()), nil
			case err := <-errChan:
				sugar.Errorw("Failed to remove services", "error", err)

				// Format output with logs and error
				var output strings.Builder

				// Add collected logs
				for _, log := range logs {
					fmt.Fprintf(&output, "%s\n", log)
				}

				fmt.Fprintf(&output, "\nFailed to remove services: %v\n", err)
				return mcp.NewToolResultText(output.String()), nil
			case <-timeoutCtx.Done():
				sugar.Warn("Service removal operation timed out")

				// Format output with logs and timeout message
				var output strings.Builder

				// Add collected logs
				for _, log := range logs {
					fmt.Fprintf(&output, "%s\n", log)
				}

				fmt.Fprintf(&output, "\nOperation timed out. The service removal might still be in progress.\n")
				return mcp.NewToolResultText(output.String()), nil
			}
		}
	})

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
