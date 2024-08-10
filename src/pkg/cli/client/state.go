package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

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
	StateDir  = filepath.Join(stateDir, "defang")
	statePath = filepath.Join(StateDir, "state.json")
	state     State
)

type State struct {
	AnonID        string
	TermsAccepted time.Time
}

func initState(path string) State {
	state := State{AnonID: uuid.NewString()}
	if bytes, err := os.ReadFile(path); err == nil {
		json.Unmarshal(bytes, &state)
	} else { // could be not found or path error
		state.write(path)
	}
	return state
}

func (state State) write(path string) error {
	if bytes, err := json.MarshalIndent(state, "", "  "); err != nil {
		return err
	} else {
		os.MkdirAll(StateDir, 0700)
		return os.WriteFile(path, bytes, 0644)
	}
}

func GetAnonID() string {
	state = initState(statePath)
	return state.AnonID
}

func AcceptTerms() error {
	state.TermsAccepted = time.Now()
	return state.write(statePath)
}

func TermsAccepted() bool {
	// Consider the terms accepted if the timestamp is within the last 24 hours
	return time.Since(state.TermsAccepted) < 24*time.Hour
}
