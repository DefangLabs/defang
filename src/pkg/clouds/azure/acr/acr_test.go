//go:build integration

package acr

import (
	"context"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
)

var testResourceGroupName = "crun-test-" + pkg.GetCurrentUser()

func TestSetUpRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}

	ctx := t.Context()

	acr := New(testResourceGroupName, "westus2")
	registryName := "defangtest" + pkg.RandomID()

	err := acr.SetUpRegistry(ctx, registryName)
	if err != nil {
		t.Fatalf("SetUpRegistry failed: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		client, err := acr.newRegistriesClient()
		if err != nil {
			t.Logf("cleanup: failed to create client: %v", err)
			return
		}
		poller, err := client.BeginDelete(ctx, acr.resourceGroupName, registryName, nil)
		if err != nil {
			t.Logf("cleanup: failed to delete registry: %v", err)
			return
		}
		_, _ = poller.PollUntilDone(ctx, nil)
	})

	if acr.LoginServer() == "" {
		t.Fatal("expected non-empty login server")
	}
	t.Logf("Registry login server: %s", acr.LoginServer())
}

func TestRunTask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}

	ctx := t.Context()

	acr := New(testResourceGroupName, "westus2")
	registryName := "defangtest" + pkg.RandomID()

	err := acr.SetUpRegistry(ctx, registryName)
	if err != nil {
		t.Fatalf("SetUpRegistry failed: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		client, err := acr.newRegistriesClient()
		if err != nil {
			return
		}
		poller, _ := client.BeginDelete(ctx, acr.resourceGroupName, registryName, nil)
		if poller != nil {
			_, _ = poller.PollUntilDone(ctx, nil)
		}
	})

	runID, err := acr.RunTask(ctx, TaskRequest{
		Image:   "alpine:latest",
		Command: []string{"echo", "hello from ACR task"},
		Timeout: 1 * time.Minute,
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	t.Logf("Run ID: %s", runID)

	t.Run("GetRunStatus", func(t *testing.T) {
		status, err := acr.GetRunStatus(ctx, runID)
		if err != nil {
			t.Fatalf("GetRunStatus failed: %v", err)
		}
		t.Logf("Status: %s (terminal=%v, success=%v)", status.Status, status.IsTerminal(), status.IsSuccess())
	})

	t.Run("GetRunLogURL", func(t *testing.T) {
		logURL, err := acr.GetRunLogURL(ctx, runID)
		if err != nil {
			t.Fatalf("GetRunLogURL failed: %v", err)
		}
		t.Logf("Log URL: %s", logURL)
	})

	t.Run("TailRunLogs", func(t *testing.T) {
		logIter, err := acr.TailRunLogs(ctx, runID)
		if err != nil {
			t.Fatalf("TailRunLogs failed: %v", err)
		}
		var lineCount int
		for line, err := range logIter {
			if err != nil {
				t.Fatalf("TailRunLogs yielded error: %v", err)
			}
			t.Logf("  %s", line)
			lineCount++
		}
		t.Logf("Total log lines: %d", lineCount)
		if lineCount == 0 {
			t.Fatal("expected at least one log line")
		}
	})
}
