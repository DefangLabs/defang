//go:integration

package aci

import (
	"context"
	"io"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/types"
)

func TestTail(t *testing.T) {
	t.SkipNow() // too slow for CI

	ctx := context.Background()

	containerInstance := NewContainerInstance(testResourceGroupName, "westeurope")

	err := containerInstance.SetUp(ctx, []types.Container{
		{
			Name:    "test-container",
			Image:   "library/alpine:latest",
			Command: []string{"sh", "-c", "sleep 3; cat /etc/hosts"},
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

	t.Cleanup(func() {
		err := containerInstance.Stop(ctx, taskID)
		if err != nil {
			t.Fatalf("Failed to stop container instance: %v", err)
		}
	})

	err = containerInstance.Tail(ctx, taskID)
	if err != io.EOF {
		t.Fatalf("Tail failed: %v", err)
	}
}
