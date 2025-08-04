package aci

import (
	"testing"

	"github.com/google/uuid"
)

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
