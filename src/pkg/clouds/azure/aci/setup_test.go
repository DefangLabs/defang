//go:build integration

package aci

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/types"
)

func TestSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}

	c := NewContainerInstance(testResourceGroupName, "westeurope")

	t.Run("SetUp", func(t *testing.T) {
		err := c.SetUp(context.Background(), []types.Container{
			{
				Name:   "test-container",
				Image:  "library/nginx:latest",
				Cpus:   1,
				Memory: 1024 * 1024 * 1024, // 1 GB in B
			},
		})
		if err != nil {
			t.Errorf("Failed to set up container instance: %v", err)
		}
	})

	t.Run("TearDown", func(t *testing.T) {
		err := c.TearDown(context.Background())
		if err != nil {
			t.Fatalf("Failed to tear down container instance: %v", err)
		}
	})
}
