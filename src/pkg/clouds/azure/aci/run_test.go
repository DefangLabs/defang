//go:build integration

package aci

import (
	"context"
	"testing"
)

func TestRun(t *testing.T) {
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
}
