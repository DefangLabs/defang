//go:integration

package aci

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
)

var testResourceGroupName = "crun-test-" + pkg.GetCurrentUser() // avoid conflict with other users in the same account

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

func TestStorage(t *testing.T) {
	c := NewContainerInstance(testResourceGroupName, "westeurope")

	storageAccountName, err := c.setUpStorageAccount(context.Background())
	if err != nil {
		t.Fatalf("Failed to set up storage account: %v", err)
	}

	foundAccountName, err := c.getStorageAccount(context.Background())
	if err != nil {
		t.Fatalf("Failed to get storage account name: %v", err)
	}
	if foundAccountName != storageAccountName {
		t.Fatalf("Expected storage account name %s, got %s", storageAccountName, foundAccountName)
	}
}
