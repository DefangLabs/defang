package aci

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/google/uuid"
)

var testResourceGroupName = "crun-test-" + pkg.GetCurrentUser() // avoid conflict with other users in the same account

func TestNewClient(t *testing.T) {
	t.Setenv("AZURE_SUBSCRIPTION_ID", uuid.NewString())

	c := NewContainerInstance(testResourceGroupName, "")

	client, err := c.newContainerGroupClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}
