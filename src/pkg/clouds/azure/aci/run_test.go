//go:build integration

package aci

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/types"
)

func TestRun(t *testing.T) {
	t.SkipNow() // too slow for CI

	ctx := context.Background()

	containerInstance := NewContainerInstance(testResourceGroupName, "westeurope")

	err := containerInstance.SetUp(ctx, []types.Container{
		{
			Name:  "test-container",
			Image: "library/alpine:latest",
		},
	})
	if err != nil {
		t.Fatalf("SetUp failed: %v", err)
	}

	t.Cleanup(func() {
		// err := containerInstance.TearDown(ctx)
		// if err != nil {
		// 	t.Fatalf("Failed to tear down container instance: %v", err)
		// }
	})

	taskID, err := containerInstance.Run(ctx, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if taskID == nil {
		t.Fatal("Expected non-nil task ID")
	}
}
