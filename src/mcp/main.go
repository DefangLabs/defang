package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
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
)

const (
	host           = "fabric-prod1.defang.dev:443"
	gitHubClientId = "7b41848ca116eac4b125" // Default GitHub OAuth app ID
)

var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelInfo,
}))

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

	// Create state directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write token to file
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	return nil
}

func validateToken(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}

	// Create a temporary client to validate token
	tempClient := client.NewGrpcClient(host, token, types.TenantName(""))

	// Try to get user info
	_, err := tempClient.WhoAmI(ctx)
	return err == nil
}

func login(ctx context.Context, grpcClient client.GrpcClient) (string, error) {
	// Start GitHub auth flow
	code, err := github.StartAuthCodeFlow(ctx, gitHubClientId)
	if err != nil {
		return "", fmt.Errorf("failed to start auth flow: %w", err)
	}

	// Exchange code for token with unrestricted access
	resp, err := grpcClient.Token(ctx, &defangv1.TokenRequest{
		AuthCode:  code,
		Tenant:    string(types.DEFAULT_TENANT),
		Scope:     nil, // nil scope = unrestricted access
		ExpiresIn: uint32(24 * time.Hour.Seconds()),
	})
	if err != nil {
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}

	return resp.AccessToken, nil
}

func getValidToken(ctx context.Context) (string, error) {
	// Try to get existing token
	token := getExistingToken(host)

	// Validate token if we have one
	if token != "" && validateToken(ctx, token) {
		return token, nil
	}

	// Create a temporary gRPC client for login
	tempClient := client.NewGrpcClient(host, "", types.TenantName(""))

	// Get token through GitHub auth flow
	token, err := login(ctx, tempClient)
	if err != nil {
		return "", fmt.Errorf("failed to login: %w", err)
	}

	// Save token to file and environment
	if err := saveToken(host, token); err != nil {
		logger.Warn("failed to save token", "error", err)
	}
	os.Setenv("DEFANG_ACCESS_TOKEN", token)

	return token, nil
}

func getServices(ctx context.Context, token string) (*defangv1.GetServicesResponse, error) {
	// Create a loader
	loader := compose.NewLoader()

	// Get project name from loader
	projectName, err := loader.LoadProjectName(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load project name: %w", err)
	}

	// Create a gRPC client with token
	grpcClient := client.NewGrpcClient(host, token, types.TenantName(""))

	// Create a provider using cli.NewProvider
	provider, err := cli.NewProvider(ctx, client.ProviderDefang, grpcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	logger.Info("Getting services for project", projectName)

	servicesResponse, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName})
	if err != nil {
		return nil, fmt.Errorf("failed to get services: %w", err)
	}

	return servicesResponse, nil
}

func main() {
	slog.SetDefault(logger)

	// Parse flags
	projectDir := flag.String("C", ".", "Project directory containing compose.yaml")
	flag.Parse()

	// Set up context
	ctx := context.Background()

	// Change to project directory
	if err := os.Chdir(*projectDir); err != nil {
		logger.Error("failed to change directory", "error", err)
		os.Exit(1)
	}

	// Get valid token
	token, err := getValidToken(ctx)
	if err != nil {
		logger.Error("failed to get valid token", "error", err)
		os.Exit(1)
	}

	// Create a new MCP server
	s := server.NewMCPServer(
		"Defang Services",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Create a tool for listing services
	servicesTool := mcp.NewTool("services",
		mcp.WithDescription("List information about services in Defang"),
		mcp.WithString("project",
			mcp.Description("Project name to filter services"),
		),
	)

	// Add the services tool handler
	s.AddTool(servicesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get services
		logger.Info("Fetching services from Defang")
		services, err := getServices(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("failed to get services: %w", err)
		}

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
	})

	// Start the server
	logger.Info("Starting Defang Services MCP server")
	if err := server.ServeStdio(s); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
