package cli

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/types/known/emptypb"
)

type grpcLogoutMockHandler struct {
	defangv1connect.UnimplementedFabricControllerHandler
	returnUnauthenticated bool
}

func (g *grpcLogoutMockHandler) RevokeToken(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
	if g.returnUnauthenticated {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func TestLogout(t *testing.T) {
	mockService := &grpcLogoutMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := context.Background()
	url := strings.TrimPrefix(server.URL, "http://")

	// Create a temporary directory for token storage
	tmpDir := t.TempDir()
	originalStateDir := client.StateDir
	client.StateDir = tmpDir
	originalTokenStore := client.TokenStore
	client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: tmpDir}
	t.Cleanup(func() {
		client.StateDir = originalStateDir
		client.TokenStore = originalTokenStore
	})

	if err := client.TokenStore.Save(client.TokenStorageName(url), "mock-token"); err != nil {
		t.Fatal(err)
	}

	// Also create a JWT token file
	jwtFile, _ := client.GetWebIdentityTokenFile(url)
	t.Logf("JWT file path: %s", jwtFile)
	err := os.WriteFile(jwtFile, []byte("mock-jwt-token"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Verify files exist before logout
	if _, err := client.TokenStore.Load(client.TokenStorageName(url)); err != nil {
		t.Fatal("Token file should exist before logout")
	}
	if _, err := os.Stat(jwtFile); os.IsNotExist(err) {
		t.Fatal("JWT file should exist before logout")
	}

	// Create a gRPC client
	grpcClient := client.NewGrpcClient(url, "mock-token", "")

	// Perform logout
	err = Logout(ctx, grpcClient, url)
	if err != nil {
		t.Fatal(err)
	}

	// Verify token file was removed
	if _, err := client.TokenStore.Load(client.TokenStorageName(url)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Token file should be removed after logout, but got error: %v", err)
	}

	// Verify JWT file was removed
	if _, err := os.Stat(jwtFile); !os.IsNotExist(err) {
		t.Errorf("JWT file should be removed after logout, but got error: %v", err)
	}
}

func TestLogoutWithoutTokenFile(t *testing.T) {
	mockService := &grpcLogoutMockHandler{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := context.Background()
	url := strings.TrimPrefix(server.URL, "http://")

	// Create a temporary directory for token storage
	tmpDir := t.TempDir()
	originalStateDir := client.StateDir
	client.StateDir = tmpDir
	t.Cleanup(func() {
		client.StateDir = originalStateDir
	})

	// Create a gRPC client
	cluster := url
	grpcClient := client.NewGrpcClient(url, "mock-token", "")

	// Perform logout without token file (should not error)
	err := Logout(ctx, grpcClient, cluster)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogoutWithUnauthenticatedError(t *testing.T) {
	mockService := &grpcLogoutMockHandler{returnUnauthenticated: true}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx := context.Background()
	url := strings.TrimPrefix(server.URL, "http://")

	// Create a temporary directory for token storage
	tmpDir := t.TempDir()
	originalStateDir := client.StateDir
	client.StateDir = tmpDir
	originalTokenStore := client.TokenStore
	client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: tmpDir}
	t.Cleanup(func() {
		client.StateDir = originalStateDir
		client.TokenStore = originalTokenStore
	})

	if err := client.TokenStore.Save(client.TokenStorageName(url), "mock-token"); err != nil {
		t.Fatal(err)
	}

	// Create a gRPC client
	grpcClient := client.NewGrpcClient(url, "mock-token", "")

	// Perform logout - should succeed even with unauthenticated error
	if err := Logout(ctx, grpcClient, url); err != nil {
		t.Fatal(err)
	}

	// Verify token file was still removed
	if _, err := client.TokenStore.Load(client.TokenStorageName(url)); !errors.Is(err, os.ErrNotExist) {
		t.Error("Token should be removed from token store after logout even with unauthenticated error")
	}
}
