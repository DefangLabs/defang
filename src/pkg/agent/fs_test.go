package agent

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCwdRootReadFile(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	require.NoError(t, os.Mkdir(filepath.Join(cwd, "app"), 0o755))
	file := filepath.Join(cwd, "app", "Dockerfile")
	require.NoError(t, os.WriteFile(file, []byte("FROM scratch"), 0o600))

	for _, path := range []string{"app/Dockerfile", file} {
		t.Run(path, func(t *testing.T) {
			root, name, err := openCwdRoot(path)
			require.NoError(t, err)
			defer root.Close()

			contents, err := root.ReadFile(name)
			require.NoError(t, err)
			assert.Equal(t, "FROM scratch", string(contents))
		})
	}
}

func TestCwdRootRejectsPathsOutsideCwd(t *testing.T) {
	parent := t.TempDir()
	cwd := filepath.Join(parent, "cwd")
	require.NoError(t, os.Mkdir(cwd, 0o755))
	t.Chdir(cwd)

	outside := filepath.Join(parent, "secret")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	for _, path := range []string{"../secret", outside} {
		t.Run(path, func(t *testing.T) {
			root, name, err := openCwdRoot(path)
			require.NoError(t, err)
			defer root.Close()

			_, err = root.ReadFile(name)
			assert.Error(t, err)
		})
	}
}

func TestCwdRootRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	cwd := filepath.Join(parent, "cwd")
	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.Mkdir(cwd, 0o755))
	require.NoError(t, os.Mkdir(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	if err := os.Symlink(outside, filepath.Join(cwd, "escape")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	t.Chdir(cwd)

	root, name, err := openCwdRoot("escape/secret")
	require.NoError(t, err)
	defer root.Close()

	_, err = root.ReadFile(name)
	assert.Error(t, err)

	_, err = fs.ReadDir(root.FS(), filepath.ToSlash(filepath.Dir(name)))
	assert.Error(t, err)

	err = fs.WalkDir(root.FS(), filepath.ToSlash(filepath.Dir(name)), func(_ string, _ fs.DirEntry, err error) error {
		return err
	})
	assert.Error(t, err)
}
