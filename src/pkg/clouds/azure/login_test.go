package azure

import (
	"context"
	"os"
	"testing"
	"time"
)

// unsetEnv removes an env var for the duration of t. Unlike t.Setenv("", ""),
// the variable is fully unset so callers using LookupEnv see ok=false.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	if v, ok := os.LookupEnv(key); ok {
		_ = os.Unsetenv(key)
		t.Cleanup(func() { _ = os.Setenv(key, v) }) //nolint:usetesting
	}
}

func TestAuthenticateMissingSubscriptionID(t *testing.T) {
	// Fully unset AZURE_SUBSCRIPTION_ID for the duration of this test —
	// t.Setenv("", ...) would leave it set-but-empty and LookupEnv returns true.
	unsetEnv(t, "AZURE_SUBSCRIPTION_ID")

	a := &Azure{}
	if err := a.Authenticate(context.Background(), false); err == nil {
		t.Error("Authenticate should fail when AZURE_SUBSCRIPTION_ID is missing")
	}
	if a.Cred != nil {
		t.Error("Cred should remain nil on error")
	}
}

func TestTryGithubOIDC_NotInGithubActions(t *testing.T) {
	// Outside a GH Actions runner the function must no-op (return nil, nil) so
	// Authenticate falls through to DefaultAzureCredential. ACTIONS_ID_TOKEN_
	// REQUEST_URL is the canonical "we're in Actions" sentinel.
	unsetEnv(t, "ACTIONS_ID_TOKEN_REQUEST_URL")
	t.Setenv("AZURE_CLIENT_ID", "client-id")
	t.Setenv("AZURE_TENANT_ID", "tenant-id")

	a := &Azure{SubscriptionID: "sub-id"}
	cred, err := a.tryGithubOIDC(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when not in Actions, got %v", err)
	}
	if cred != nil {
		t.Error("expected nil credential when not in Actions")
	}
}

func TestTryGithubOIDC_MissingClientID(t *testing.T) {
	// Even when ACTIONS_ID_TOKEN_REQUEST_URL is set, AZURE_CLIENT_ID is required
	// to know which UAMI to federate as. Without it, fall through silently.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "https://example.invalid")
	unsetEnv(t, "AZURE_CLIENT_ID")
	t.Setenv("AZURE_TENANT_ID", "tenant-id")

	a := &Azure{SubscriptionID: "sub-id"}
	cred, err := a.tryGithubOIDC(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when AZURE_CLIENT_ID is missing, got %v", err)
	}
	if cred != nil {
		t.Error("expected nil credential when AZURE_CLIENT_ID is missing")
	}
}

func TestTryGithubOIDC_MissingTenantID(t *testing.T) {
	// AZURE_TENANT_ID identifies which Entra tenant to exchange the OIDC
	// token at; without it the federation has no destination. Fall through silently.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "https://example.invalid")
	t.Setenv("AZURE_CLIENT_ID", "client-id")
	unsetEnv(t, "AZURE_TENANT_ID")

	a := &Azure{SubscriptionID: "sub-id"}
	cred, err := a.tryGithubOIDC(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when AZURE_TENANT_ID is missing, got %v", err)
	}
	if cred != nil {
		t.Error("expected nil credential when AZURE_TENANT_ID is missing")
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
