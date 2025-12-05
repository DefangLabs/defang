package command

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
)

func TestInitializeTailCmd(t *testing.T) {
	t.Run("", func(t *testing.T) {
		for _, cmd := range RootCmd.Commands() {
			if cmd.Use == "logs" {
				cmd.Execute()
				return
			}
		}
	})
}

func TestHandleTailAndMonitorErr_ContextCanceled(t *testing.T) {
	// Create a canceled context to simulate Ctrl-C
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create a deployment failure error
	err := cliClient.ErrDeploymentFailed{
		Service: "test-service",
		Message: "deployment failed",
	}

	// Create a debug config
	debugConfig := cli.DebugConfig{
		Deployment: "test-deployment",
		Project:    &compose.Project{Name: "test-project"},
	}

	// This should not panic or prompt for debugging
	handleTailAndMonitorErr(ctx, err, nil, debugConfig)
}

func TestHandleTailAndMonitorErr_NoContextCancellation(t *testing.T) {
	// Create a normal context (not canceled)
	ctx := context.Background()

	// Create a deployment failure error
	err := cliClient.ErrDeploymentFailed{
		Service: "test-service",
		Message: "deployment failed",
	}

	// Create a debug config
	debugConfig := cli.DebugConfig{
		Deployment: "test-deployment",
		Project:    &compose.Project{Name: "test-project"},
	}

	// Set NonInteractive to true to avoid actually prompting for debugging
	oldNonInteractive := global.NonInteractive
	global.NonInteractive = true
	defer func() { global.NonInteractive = oldNonInteractive }()

	// This should print a hint but not prompt for interactive debugging
	handleTailAndMonitorErr(ctx, err, nil, debugConfig)
}

func TestHandleComposeUpErr_ContextCanceled(t *testing.T) {
	// Create a canceled context to simulate Ctrl-C
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create a generic error
	err := errors.New("some error during compose up")

	// Create a test project
	project := &compose.Project{Name: "test-project"}

	// This should return the error without prompting for debugging
	result := handleComposeUpErr(ctx, err, project, nil)

	if result != err {
		t.Errorf("Expected error to be returned as-is, got: %v", result)
	}
}
