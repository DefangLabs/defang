package cli

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		currentToken := os.Getenv("DEFANG_ACCESS_TOKEN")
		if currentToken != expectedToken {
			os.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)
		}
		defer os.Setenv("DEFANG_ACCESS_TOKEN", currentToken)

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from file", func(t *testing.T) {
		expectedToken := "file-token"
		tokenFile := getTokenFile(fabric)
		os.WriteFile(tokenFile, []byte(expectedToken), 0600)

		currentToken := os.Getenv("DEFANG_ACCESS_TOKEN")
		os.Unsetenv("DEFANG_ACCESS_TOKEN")

		defer func() {
			os.Remove(tokenFile)
			os.Setenv("DEFANG_ACCESS_TOKEN", currentToken)
		}()

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
	tokenFile := getTokenFile(fabric)

	defer func() {
		githubAuthService = temp
		os.Remove(tokenFile)
	}()

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

func TestNonInteractiveLogin(t *testing.T) {
	ctx := context.Background()
	mockClient := client.MockFabricClient{}
	fabric := "test.defang.dev"

	t.Run("Access Token should be saved when login succeeds", func(t *testing.T) {
		requestUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
		requestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
		if len(requestUrl) == 0 || len(requestToken) == 0 {
			t.Skip("ACTIONS_ID_TOKEN_REQUEST_URL not set")
		}

		err := NonInteractiveLogin(ctx, mockClient, fabric)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		tokenFile := getTokenFile(fabric)
		savedToken, err := os.ReadFile(tokenFile)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(savedToken) == 0 {
			t.Fatalf("expected token, got none")
		}

		defer os.Remove(tokenFile)
	})

	t.Run("Login fails in the case that GitHub Actions info is not set", func(t *testing.T) {
		err := NonInteractiveLogin(ctx, mockClient, fabric)
		if err != nil &&
			err.Error() != "non-interactive login failed: ACTIONS_ID_TOKEN_REQUEST_URL or ACTIONS_ID_TOKEN_REQUEST_TOKEN not set" {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
