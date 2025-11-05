package cluster

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"
	os.Unsetenv("DEFANG_ACCESS_TOKEN")

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
		tokenFile := GetTokenFile(fabric)
		err := os.WriteFile(tokenFile, []byte(expectedToken), 0600)
		assert.NoError(t, err)

		t.Cleanup(func() {
			os.Remove(tokenFile)
		})

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got %s", expectedToken, accessToken)
		}
	})
}
