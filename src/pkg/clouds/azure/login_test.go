package azure

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestAuthenticateMissingSubscriptionID(t *testing.T) {
	// Fully unset AZURE_SUBSCRIPTION_ID for the duration of this test —
	// t.Setenv("", ...) would leave it set-but-empty and LookupEnv returns true.
	if v, ok := os.LookupEnv("AZURE_SUBSCRIPTION_ID"); ok {
		_ = os.Unsetenv("AZURE_SUBSCRIPTION_ID")
		t.Cleanup(func() { _ = os.Setenv("AZURE_SUBSCRIPTION_ID", v) }) //nolint:usetesting
	}

	a := &Azure{}
	if err := a.Authenticate(context.Background(), false); err == nil {
		t.Error("Authenticate should fail when AZURE_SUBSCRIPTION_ID is missing")
	}
	if a.Cred != nil {
		t.Error("Cred should remain nil on error")
	}
}

func TestAuthenticateNonInteractiveFailsWithInvalidSubscription(t *testing.T) {
	// An unknown subscription ID: the test call to ARM fails (either because
	// the subscription doesn't exist or because the caller has no credentials
	// at all). Non-interactive mode must return an error instead of prompting.
	a := &Azure{SubscriptionID: "00000000-0000-0000-0000-000000000000"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Authenticate(ctx, false); err == nil {
		t.Error("Authenticate(interactive=false) should fail with invalid subscription")
	}
}
