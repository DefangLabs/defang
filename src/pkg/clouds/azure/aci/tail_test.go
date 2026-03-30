//go:build integration

package aci

import (
	"context"
	"io"
	"testing"
)

func TestTail(t *testing.T) {
	t.SkipNow() // too slow for CI

	ctx := context.Background()

	containerInstance := NewContainerInstance(testResourceGroupName, "westeurope")

	err := containerInstance.SetUpResourceGroup(ctx)
	if err != nil {
		t.Fatalf("SetUpResourceGroup failed: %v", err)
	}

	t.Cleanup(func() {
		// err := containerInstance.TearDown(ctx)
		// if err != nil {
		// 	t.Fatalf("Failed to tear down container instance: %v", err)
		// }
	})

	taskID, err := containerInstance.Run(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if taskID == nil {
		t.Fatal("Expected non-nil task ID")
	}

	t.Cleanup(func() {
		err := containerInstance.Stop(ctx, taskID)
		if err != nil {
			t.Fatalf("Failed to stop container instance: %v", err)
		}
	})

	err = containerInstance.Tail(ctx, taskID, "")
	if err != io.EOF {
		t.Fatalf("Tail failed: %v", err)
	}
}
