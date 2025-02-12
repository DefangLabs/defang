package cli

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		t.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from file", func(t *testing.T) {
		expectedToken := "file-token"
		tokenFile := getTokenFile(fabric)
		os.WriteFile(tokenFile, []byte(expectedToken), 0600)

		t.Cleanup(func() {
			os.Remove(tokenFile)
		})

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})
}

type MockGitHubAuthService struct {
	GitHubAuthService
	MockLogin func(ctx context.Context, client client.FabricClient, gitHubClientId, fabric string) (string, error)
}

func (g MockGitHubAuthService) login(
	ctx context.Context, client client.FabricClient, gitHubClientId, fabric string,
) (string, error) {
	return g.MockLogin(ctx, client, gitHubClientId, fabric)
}

func TestInteractiveLogin(t *testing.T) {
	temp := githubAuthService
	accessToken := "test-token"
	fabric := "test.defang.dev"
	// use a temp dir for the token file
	t.setEnv("XDG_STATE_HOME", t.TempDir())
	tokenFile := getTokenFile(fabric)

	t.Cleanup(func() {
		githubAuthService = temp
	})

	t.Run("Expect accessToken to be stored when InteractiveLogin() succeeds", func(t *testing.T) {
		githubAuthService = MockGitHubAuthService{
			MockLogin: func(
				ctx context.Context, client client.FabricClient, gitHubClientId, fabric string,
			) (string, error) {
				return accessToken, nil
			},
		}

		err := InteractiveLogin(context.Background(), client.MockFabricClient{}, "github-client-id", fabric)
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
		githubAuthService = MockGitHubAuthService{
			MockLogin: func(ctx context.Context, client client.FabricClient, gitHubClientId, fabric string) (string, error) {
				return "", errors.New("test-error")
			},
		}

		err := InteractiveLogin(context.Background(), client.MockFabricClient{}, "github-client-id", fabric)
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
	ctx := context.Background()
	mockClient := &MockForNonInteractiveLogin{}
	fabric := "test.defang.dev"

	t.Run("Expect accessToken to be stored when NonInteractiveLogin() succeeds", func(t *testing.T) {
		requestUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		if len(requestUrl) == 0 || len(requestToken) == 0 {
			t.Skip("ACTIONS_ID_TOKEN_REQUEST_URL not set")
		}

		err := NonInteractiveLogin(ctx, mockClient, fabric)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// use a temp dir for the token file
		t.setEnv("XDG_STATE_HOME", t.TempDir())
		tokenFile := getTokenFile(fabric)
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
			err := NonInteractiveLogin(ctx, mockClient, fabric)
			if err != nil &&
				err.Error() != "non-interactive login failed: ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN not set" {
				t.Fatalf("expected no error, got %v", err)
			}
		},
	)
}
