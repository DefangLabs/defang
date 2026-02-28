package cli

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
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
	fabricAddr := strings.TrimPrefix(server.URL, "http://")

	// Create a temporary directory for token storage
	tmpDir := t.TempDir()
	originalStateDir := client.StateDir
	client.StateDir = tmpDir
	t.Cleanup(func() {
		client.StateDir = originalStateDir
	})

	// Create a mock token file
	tokenFile := client.GetTokenFile(fabricAddr)
	t.Logf("Token file path: %s", tokenFile)
	err := os.MkdirAll(filepath.Dir(tokenFile), 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(tokenFile, []byte("mock-token"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Also create a JWT token file
	jwtFile := tokenFile + ".jwt"
	t.Logf("JWT file path: %s", jwtFile)
	err = os.WriteFile(jwtFile, []byte("mock-jwt-token"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Verify files exist before logout
	if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
		t.Fatal("Token file should exist before logout")
	}
	if _, err := os.Stat(jwtFile); os.IsNotExist(err) {
		t.Fatal("JWT file should exist before logout")
	}

	// Create a gRPC client
	grpcClient := client.NewGrpcClient(fabricAddr, "mock-token", "")

	// Perform logout
	err = Logout(ctx, grpcClient, fabricAddr)
	if err != nil {
		t.Fatal(err)
	}

	// Verify token file was removed
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
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
	fabricAddr := strings.TrimPrefix(server.URL, "http://")

	// Create a temporary directory for token storage
	tmpDir := t.TempDir()
	originalStateDir := client.StateDir
	client.StateDir = tmpDir
	t.Cleanup(func() {
		client.StateDir = originalStateDir
	})

	// Create a gRPC client
	grpcClient := client.NewGrpcClient(fabricAddr, "mock-token", "")

	// Perform logout without token file (should not error)
	err := Logout(ctx, grpcClient, fabricAddr)
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
	fabricAddr := strings.TrimPrefix(server.URL, "http://")

	// Create a temporary directory for token storage
	tmpDir := t.TempDir()
	originalStateDir := client.StateDir
	client.StateDir = tmpDir
	t.Cleanup(func() {
		client.StateDir = originalStateDir
	})

	// Create a mock token file
	tokenFile := client.GetTokenFile(fabricAddr)
	err := os.MkdirAll(filepath.Dir(tokenFile), 0700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(tokenFile, []byte("mock-token"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// Create a gRPC client
	grpcClient := client.NewGrpcClient(fabricAddr, "mock-token", "")

	// Perform logout - should succeed even with unauthenticated error
	err = Logout(ctx, grpcClient, fabricAddr)
	if err != nil {
		t.Fatal(err)
	}

	// Verify token file was still removed
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Error("Token file should be removed after logout even with unauthenticated error")
	}
}
