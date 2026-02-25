package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/tokenstore"
	"github.com/stretchr/testify/require"
)

func TestGetExistingToken(t *testing.T) {
	fabric := "test.defang.dev"
	os.Unsetenv("DEFANG_ACCESS_TOKEN")
	oldTokenStore := TokenStore
	stateDir := t.TempDir()
	TokenStore = &tokenstore.LocalDirTokenStore{Dir: stateDir}
	t.Cleanup(func() {
		TokenStore = oldTokenStore
	})

	t.Run("Get access token from environmental variable", func(t *testing.T) {
		expectedToken := "env-token"
		t.Setenv("DEFANG_ACCESS_TOKEN", expectedToken)

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})

	t.Run("Get access token from store", func(t *testing.T) {
		expectedToken := "file-token"
		err := TokenStore.Save(TokenStorageName(fabric), expectedToken)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := TokenStore.Delete(TokenStorageName(fabric))
			require.NoError(t, err)
		})

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})

	t.Run("Ignore legacy tenant-prefixed token file", func(t *testing.T) {
		require.NoError(t, TokenStore.Save("legacy@"+fabric, "legacy-token"))

		accessToken := GetExistingToken("legacy@" + fabric)
		if accessToken != "" {
			t.Errorf("expected empty token when legacy file is present, got: %q", accessToken)
		}
	})

	t.Run("TokenStore is backwards compatible with older token files", func(t *testing.T) {
		expectedToken := "file-token"
		require.NoError(t, os.WriteFile(filepath.Join(stateDir, TokenStorageName(fabric)), []byte(expectedToken), 0600))

		accessToken := GetExistingToken(fabric)
		if accessToken != expectedToken {
			t.Errorf("expected %s, got: %s", expectedToken, accessToken)
		}
	})
}
