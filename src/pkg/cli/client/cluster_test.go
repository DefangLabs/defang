package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"
	os.Unsetenv("DEFANG_ACCESS_TOKEN")

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		t.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from file", func(t *testing.T) {
		expectedToken := "file-token"
		tokenFile := GetTokenFile(fabric)
		err := os.WriteFile(tokenFile, []byte(expectedToken), 0600)
		require.NoError(t, err)

		t.Cleanup(func() {
			os.Remove(tokenFile)
		})

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})

	t.Run("Ignore legacy tenant-prefixed token file", func(t *testing.T) {
		legacyFile := filepath.Join(
			filepath.Dir(GetTokenFile(fabric)), // same dir tokens normally live in
			"legacy@"+fabric,
		)
		require.NoError(t, os.MkdirAll(filepath.Dir(legacyFile), 0o700))
		require.NoError(t, os.WriteFile(legacyFile, []byte("legacy-token"), 0o600))

		t.Cleanup(func() {
			os.Remove(legacyFile)
		})

		accessToken := GetExistingToken("legacy@" + fabric)
		if accessToken != "" {
			t.Errorf("expected empty token when legacy file is present, got: %q", accessToken)
		}
	})
}
