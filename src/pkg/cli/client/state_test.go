package client

import (
	"os"
	"runtime"
	"testing"
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
