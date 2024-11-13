//go:build !windows

package command

import (
	"os"
	"path/filepath"
	"strings"
)

func WriteHiddenFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(getHiddenPath(path), data, perm)
}

func ReadHiddenFile(path string) ([]byte, error) {
	return os.ReadFile(getHiddenPath(path))
}

func getHiddenPath(path string) string {
	dir, name := filepath.Split(path)
	if !strings.HasPrefix(name, ".") {
		name = "." + name
	}
	return filepath.Join(dir, name)
}
