package cli

import (
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		if os.Getenv("DEFANG_ACCESS_TOKEN") != expectedToken {
			os.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)
		}
		defer os.Unsetenv("DEFANG_ACCESS_TOKEN")

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from environmental token file", func(t *testing.T) {
		expectedToken := "file-token"
		tokenFile := getTokenFile(fabric)
		os.WriteFile(tokenFile, []byte(expectedToken), 0600)
		defer os.Remove(tokenFile)
		os.Unsetenv("DEFANG_ACCESS_TOKEN")

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})
}

func TestInteractiveLogin(t *testing.T) {
	t.Run("TestInteractiveLogin", func(t *testing.T) {
		t.Skip("To implement TestInteractiveLogin, need to change ")
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

func TestSaveAccessToken(t *testing.T) {
	fabric := "test.defang.dev"
	token := "test-token"
	tokenFile := getTokenFile(fabric)

	t.Run("Save access token successfully", func(t *testing.T) {
		err := saveAccessToken(fabric, token)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		savedToken, _ := os.ReadFile(tokenFile)
		if string(savedToken) != token {
			t.Errorf("expected token %s, got %s", token, string(savedToken))
		}
		defer os.Remove(tokenFile)
	})
}
