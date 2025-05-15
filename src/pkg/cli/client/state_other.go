//go:build !windows

package client

import (
	"os"
	"path/filepath"
)

func userStateDir() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return stateHome, nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".local", "state"), err
}
