package client

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStateDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TestStateDir() not implemented for Windows")
	}
	os.Setenv("HOME", "/home/user")
	stateDir, err := userStateDir()
	if err != nil {
		t.Fatalf("userStateDir() returned error: %v", err)
	}
	if stateDir != "/home/user/.local/state" {
		t.Errorf("userStateDir() returned unexpected directory: %v", stateDir)
	}
}

func TestInitState(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "state.json")
	state := initState(tmp)
	if state.AnonID == "" {
		t.Errorf("initState() returned empty AnonID")
	}
	// 2nd call should read from same file
	state2 := initState(tmp)
	if state2.AnonID != state.AnonID {
		t.Errorf("initState() returned different AnonID on 2nd call")
	}
}

func TestTerms(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "state.json")
	state := initState(tmp)
	if !state.TermsAcceptedAt.IsZero() {
		t.Errorf("initState() returned non-zero TermsAccepted")
	}
	if state.termsAccepted() {
		t.Errorf("TermsAccepted() returned true, expected false")
	}
	if err := state.acceptTerms(); err != nil {
		t.Errorf("AcceptTerms() returned error: %v", err)
	}
	if !state.termsAccepted() {
		t.Errorf("TermsAccepted() returned false, expected true")
	}
	// Old acceptance should not count
	state.TermsAcceptedAt = state.TermsAcceptedAt.Add(-25 * time.Hour)
	if state.termsAccepted() {
		t.Errorf("TermsAccepted() returned true, expected false")
	}
}
