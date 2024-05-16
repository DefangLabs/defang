package client

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/DefangLabs/defang/src/pkg"
)

func userStateDir() (string, error) {
	if runtime.GOOS == "windows" {
		return os.UserCacheDir()
	} else {
		home, err := os.UserHomeDir()
		return pkg.Getenv("XDG_STATE_HOME", filepath.Join(home, ".local/state")), err
	}
}

var (
	stateDir, _ = userStateDir()
	// StateDir is the directory where the state file is stored
	StateDir = filepath.Join(stateDir, "defang")
)

type State struct {
	AnonID string
}
