//go:build integration

package aci

import (
	"context"
	"testing"
)

func TestSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}

	c := NewContainerInstance(testResourceGroupName, "westeurope")

	t.Run("SetUpResourceGroup", func(t *testing.T) {
		err := c.SetUpResourceGroup(context.Background())
		if err != nil {
			t.Errorf("Failed to set up resource group: %v", err)
		}
	})

	t.Run("TearDown", func(t *testing.T) {
		err := c.TearDown(context.Background())
		if err != nil {
			t.Fatalf("Failed to tear down container instance: %v", err)
		}
	})
}
