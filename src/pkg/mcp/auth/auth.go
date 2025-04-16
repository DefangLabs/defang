package auth

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/mcp/logger"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const (
	Host           = "fabric-prod1.defang.dev:443"
	GitHubClientId = "7b41848ca116eac4b125" // Default GitHub OAuth app ID
)

func GetTokenFile(fabric string) string {
	if host, _, _ := net.SplitHostPort(fabric); host != "" {
		fabric = host
	}
	return filepath.Join(client.StateDir, fabric)
}

func GetExistingToken() string {
	// First check environment variable
	accessToken := os.Getenv("DEFANG_ACCESS_TOKEN")
	if accessToken != "" {
		return accessToken
	}

	// Then check token file
	tokenFile := GetTokenFile(Host)
	all, err := os.ReadFile(tokenFile)
	if err == nil {
		return string(all)
	}

	return ""
}

func SaveToken(fabric, token string) error {
	tokenFile := GetTokenFile(fabric)

	logger.Sugar.Infow("Saving token to file", "file", tokenFile)

	// Create state directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0700); err != nil {
		logger.Sugar.Errorw("Failed to create state directory", "error", err)
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write token to file
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		logger.Sugar.Errorw("Failed to save token", "error", err)
		return fmt.Errorf("failed to save token: %w", err)
	}

	logger.Sugar.Info("Token saved successfully")
	return nil
}

func ValidateToken(ctx context.Context, token string) bool {
	if token == "" {
		logger.Sugar.Debug("Empty token provided for validation")
		return false
	}

	logger.Sugar.Debug("Validating token")

	// Create a temporary client to validate token
	tempClient := client.NewGrpcClient(Host, token, types.TenantName(""))

	// Try to get user info
	_, err := tempClient.WhoAmI(ctx)
	if err != nil {
		logger.Sugar.Debugw("Token validation failed", "error", err)
		return false
	}

	logger.Sugar.Debug("Token validated successfully")
	return true
}

func Login(ctx context.Context, grpcClient client.GrpcClient) (string, error) {
	logger.Sugar.Info("Starting GitHub authentication flow")

	// Start GitHub auth flow
	code, err := github.StartAuthCodeFlow(ctx, GitHubClientId)
	if err != nil {
		logger.Sugar.Errorw("Failed to start auth flow", "error", err)
		return "", fmt.Errorf("failed to start auth flow: %w", err)
	}

	logger.Sugar.Info("Successfully obtained GitHub auth code, exchanging for token")

	// Exchange code for token with unrestricted access
	resp, err := grpcClient.Token(ctx, &defangv1.TokenRequest{
		AuthCode:  code,
		Tenant:    string(types.DEFAULT_TENANT),
		Scope:     nil, // nil scope = unrestricted access
		ExpiresIn: uint32(24 * time.Hour.Seconds()),
	})
	if err != nil {
		logger.Sugar.Errorw("Failed to exchange code for token", "error", err)
		return "", fmt.Errorf("failed to exchange code for token: %w", err)
	}

	logger.Sugar.Info("Successfully obtained access token")
	return resp.AccessToken, nil
}

func GetValidTokenAndSave(ctx context.Context) (string, error) {
	logger.Sugar.Info("Getting valid token")

	// Try to get existing token
	token := GetExistingToken()

	// Validate token if we have one
	if token != "" {
		logger.Sugar.Debug("Found existing token, validating")
		if ValidateToken(ctx, token) {
			logger.Sugar.Info("Using existing valid token")
			return token, nil
		}
		logger.Sugar.Info("Existing token is invalid, getting new token")
	} else {
		logger.Sugar.Info("No existing token found, getting new token")
	}

	// Create a temporary gRPC client for login
	tempClient := client.NewGrpcClient(Host, "", types.TenantName(""))

	// Get token through GitHub auth flow
	token, err := Login(ctx, tempClient)
	if err != nil {
		logger.Sugar.Errorw("Failed to login", "error", err)
		return "", fmt.Errorf("failed to login: %w", err)
	}

	// Save token to file and environment
	if err := SaveToken(Host, token); err != nil {
		logger.Sugar.Warnw("Failed to save token", "error", err)
	}
	os.Setenv("DEFANG_ACCESS_TOKEN", token)
	logger.Sugar.Info("Token saved to environment variable")

	return token, nil
}
