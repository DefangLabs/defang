package command

import (
	"context"
	"testing"
)

func TestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	err := testCommand([]string{"version"})
	if err != nil {
		t.Fatalf("Version() failed: %v", err)
	}
}

func testCommand(args []string) error {
	ctx := context.Background()
	SetupCommands("test")
	RootCmd.SetArgs(args)
	return RootCmd.ExecuteContext(ctx)
}
