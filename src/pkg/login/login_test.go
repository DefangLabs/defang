package login

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"
	os.Unsetenv("DEFANG_ACCESS_TOKEN")

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		t.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)

		accessToken := cluster.GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from file", func(t *testing.T) {
		expectedToken := "file-token"
		tokenFile := cluster.GetTokenFile(fabric)
		os.WriteFile(tokenFile, []byte(expectedToken), 0600)

		t.Cleanup(func() {
			os.Remove(tokenFile)
		})

		accessToken := cluster.GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})
}

type mockGitHubAuthService struct {
	accessToken string
	err         error
}

func (g mockGitHubAuthService) login(ctx context.Context, client client.FabricClient, fabric string, prompt LoginFlow, mcpClient string) (string, error) {
	return g.accessToken, g.err
}

func TestInteractiveLogin(t *testing.T) {
	prevGithubAuthService := authService
	accessToken := "test-token"
	fabric := "test.defang.dev"
	// use a temp dir for the token file
	prevStateDir := client.StateDir
	client.StateDir = filepath.Join(t.TempDir(), "defang")

	t.Cleanup(func() {
		authService = prevGithubAuthService
		client.StateDir = prevStateDir
	})

	tokenFile := cluster.GetTokenFile(fabric)

	t.Run("Expect accessToken to be stored when InteractiveLogin() succeeds", func(t *testing.T) {
		authService = mockGitHubAuthService{accessToken: accessToken}

		err := InteractiveLogin(t.Context(), client.MockFabricClient{}, fabric)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		savedToken, err := os.ReadFile(tokenFile)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if string(savedToken) != accessToken {
			t.Errorf("expected %s, got %s", accessToken, string(savedToken))
		}
	})

	t.Run("Expect error when InteractiveLogin fails", func(t *testing.T) {
		authService = mockGitHubAuthService{err: errors.New("test-error")}

		err := InteractiveLogin(t.Context(), client.MockFabricClient{}, fabric)
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
		prevStateDir := client.StateDir
		client.StateDir = filepath.Join(t.TempDir(), "defang")

		t.Cleanup(func() { client.StateDir = prevStateDir })

		err := NonInteractiveGitHubLogin(ctx, mockClient, fabric)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		tokenFile := cluster.GetTokenFile(fabric)
		savedToken, err := os.ReadFile(tokenFile)
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
