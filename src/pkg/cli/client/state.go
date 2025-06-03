package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var (
	stateDir, _ = userStateDir()
	// StateDir is the directory where the state file is stored
	StateDir  = filepath.Join(stateDir, "defang")
	statePath = filepath.Join(StateDir, "state.json")
	state     State
)

type State struct {
	AnonID          string
	TermsAcceptedAt time.Time
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
		return os.WriteFile(path, bytes, 0600)
	}
}

func (state *State) acceptTerms() error {
	state.TermsAcceptedAt = time.Now()
	return state.write(statePath)
}

func (state State) termsAccepted() bool {
	// Consider the terms accepted if the timestamp is within the last 24 hours
	return time.Since(state.TermsAcceptedAt) < 24*time.Hour
}

func GetAnonID() string {
	state = initState(statePath)
	return state.AnonID
}

func AcceptTerms() error {
	return state.acceptTerms()
}

func TermsAccepted() bool {
	return state.termsAccepted()
}
