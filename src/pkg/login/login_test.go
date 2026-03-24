package login

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"
	os.Unsetenv("DEFANG_ACCESS_TOKEN")

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		t.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)

		accessToken := client.GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from token store", func(t *testing.T) {
		expectedToken := "file-token"
		originalTokenStore := client.TokenStore
		client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: t.TempDir()}
		client.TokenStore.Save(client.TokenStorageName(fabric), expectedToken)

		t.Cleanup(func() {
			client.TokenStore = originalTokenStore
		})

		accessToken := client.GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})
}

type mockGitHubAuthService struct {
	accessToken string
	err         error
}

func (g mockGitHubAuthService) login(ctx context.Context, fabricAddr string, prompt LoginFlow, mcpClient string) (string, error) {
	return g.accessToken, g.err
}

func TestInteractiveLogin(t *testing.T) {
	prevGithubAuthService := authService
	accessToken := "test-token"
	fabric := "test.defang.dev"
	// use a temp dir for the token file
	prevStateDir := client.StateDir
	client.StateDir = filepath.Join(t.TempDir(), "defang")
	originalTokenStore := client.TokenStore
	client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: t.TempDir()}

	t.Cleanup(func() {
		authService = prevGithubAuthService
		client.StateDir = prevStateDir
		client.TokenStore = originalTokenStore
	})

	t.Run("Expect accessToken to be stored when InteractiveLogin() succeeds", func(t *testing.T) {
		authService = mockGitHubAuthService{accessToken: accessToken}

		err := InteractiveLogin(t.Context(), fabric)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		savedToken, err := client.TokenStore.Load(client.TokenStorageName(fabric))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if savedToken != accessToken {
			t.Errorf("expected %s, got: %s", accessToken, savedToken)
		}
	})

	t.Run("Expect error when InteractiveLogin fails", func(t *testing.T) {
		authService = mockGitHubAuthService{err: errors.New("test-error")}

		err := InteractiveLogin(t.Context(), fabric)
		if err == nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

type MockForNonInteractiveLogin struct {
	client.MockFabricClient
}

func (m MockForNonInteractiveLogin) Token(ctx context.Context, req *defangv1.TokenRequest) (*defangv1.TokenResponse, error) {
	return &defangv1.TokenResponse{AccessToken: "accessToken"}, nil
}

func TestNonInteractiveLogin(t *testing.T) {
	ctx := t.Context()
	mockClient := &MockForNonInteractiveLogin{}
	fabric := "test.defang.dev"

	t.Run("Expect accessToken to be stored when NonInteractiveLogin() succeeds", func(t *testing.T) {
		requestUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		if len(requestUrl) == 0 || len(requestToken) == 0 {
			t.Skip("ACTIONS_ID_TOKEN_REQUEST_URL not set")
		}

		// use a prevStateDir dir for the token file
		tempDir := t.TempDir()
		prevStateDir := client.StateDir
		client.StateDir = filepath.Join(tempDir, "defang")
		originalTokenStore := client.TokenStore
		client.TokenStore = &tokenstore.LocalDirTokenStore{Dir: client.StateDir}

		t.Cleanup(func() {
			client.StateDir = prevStateDir
			client.TokenStore = originalTokenStore
		})

		err := NonInteractiveGitHubLogin(ctx, mockClient, fabric)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		savedToken, err := client.TokenStore.Load(client.TokenStorageName(fabric) + ".jwt")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(savedToken) == 0 {
			t.Fatalf("expected token, got none")
		}
	})

	t.Run("Expect error when NonInteractiveLogin() fails in the case that GitHub Actions info is not set",
		func(t *testing.T) {
			err := NonInteractiveGitHubLogin(ctx, mockClient, fabric)
			if err != nil &&
				err.Error() != "non-interactive login failed: ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN not set" {
				t.Fatalf("expected no error, got %v", err)
			}
		},
	)
}
