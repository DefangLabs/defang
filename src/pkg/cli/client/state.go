package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/google/uuid"
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

	GetAnonID = func() string {
		state := State{AnonID: uuid.NewString()}

		// Restore anonID from config file
		statePath := filepath.Join(StateDir, "state.json")
		if bytes, err := os.ReadFile(statePath); err == nil {
			json.Unmarshal(bytes, &state)
		} else { // could be not found or path error
			if bytes, err := json.MarshalIndent(state, "", "  "); err == nil {
				os.MkdirAll(StateDir, 0700)
				os.WriteFile(statePath, bytes, 0644)
			}
		}
		return state.AnonID
	}
)

type State struct {
	AnonID string
}
