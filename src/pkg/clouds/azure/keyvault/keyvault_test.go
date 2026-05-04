package keyvault

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

type fakeCred struct {
	tok string
	err error
}

func (f fakeCred) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if f.err != nil {
		return azcore.AccessToken{}, f.err
	}
	return azcore.AccessToken{Token: f.tok, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func useFakeCred(t *testing.T, tok string, gerr error) {
	t.Helper()
	orig := azure.NewCredsFunc
	azure.NewCredsFunc = func(_ azure.Azure) (azcore.TokenCredential, error) {
		return fakeCred{tok: tok, err: gerr}, nil
	}
	t.Cleanup(func() { azure.NewCredsFunc = orig })
}

func TestVaultName(t *testing.T) {
	name := VaultName("my-rg", "sub-id")
	if !strings.HasPrefix(name, "defang-config-") {
		t.Errorf("VaultName = %q, want defang-config- prefix", name)
	}
	if len(name) > 24 {
		t.Errorf("VaultName %q exceeds 24 chars", name)
	}
	// Deterministic.
	if VaultName("my-rg", "sub-id") != name {
		t.Error("VaultName is not deterministic")
	}
	// Different inputs produce different names.
	if VaultName("other-rg", "sub-id") == name {
		t.Error("VaultName collision for different resource group")
	}
	if VaultName("my-rg", "other-sub") == name {
		t.Error("VaultName collision for different subscription")
	}
}

func TestVaultURL(t *testing.T) {
	got := VaultURL("kv-abc123")
	want := "https://kv-abc123.vault.azure.net"
	if got != want {
		t.Errorf("VaultURL = %q, want %q", got, want)
	}
}

func TestToSecretName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"FOO", "FOO"},
		{"/Defang", "Defang"},
		{"/Defang/myapp/test/POSTGRES_PASSWORD", "Defang--myapp--test--POSTGRES-PASSWORD"},
		{"foo_bar", "foo-bar"},
		{"foo/bar", "foo--bar"},
	}
	for _, tt := range tests {
		if got := ToSecretName(tt.in); got != tt.want {
			t.Errorf("ToSecretName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNew(t *testing.T) {
	kv := New("rg-name", azure.Azure{Location: azure.LocationWestUS2, SubscriptionID: "sub-id"})
	if kv == nil {
		t.Fatal("New returned nil")
	}
	if kv.SubscriptionID != "sub-id" {
		t.Errorf("SubscriptionID = %q, want sub-id", kv.SubscriptionID)
	}
	if kv.Location != azure.LocationWestUS2 {
		t.Errorf("Location = %q, want westus2", kv.Location)
	}
	if kv.resourceGroupName != "rg-name" {
		t.Errorf("resourceGroupName = %q, want rg-name", kv.resourceGroupName)
	}
}

func TestSecretURL(t *testing.T) {
	kv := &KeyVault{vaultURL: "https://kv-abc.vault.azure.net"}
	got := kv.SecretURL("my-secret")
	want := "https://kv-abc.vault.azure.net/secrets/my-secret"
	if got != want {
		t.Errorf("SecretURL = %q, want %q", got, want)
	}
}

func TestVaultNameAndURLFields(t *testing.T) {
	kv := New("rg", azure.Azure{Location: azure.LocationWestUS2, SubscriptionID: "sub"})
	// VaultName and vaultURL are populated by SetUp; zero values before that.
	if kv.VaultName != "" {
		t.Errorf("VaultName before SetUp = %q, want empty", kv.VaultName)
	}
	if kv.vaultURL != "" {
		t.Errorf("vaultURL before SetUp = %q, want empty", kv.vaultURL)
	}
	// Simulate SetUp populating fields.
	kv.VaultName = VaultName(kv.resourceGroupName, kv.SubscriptionID)
	kv.vaultURL = VaultURL(kv.VaultName)
	if kv.vaultURL == "" {
		t.Error("vaultURL should be populated")
	}
	if got := kv.SecretURL("foo"); got != kv.vaultURL+"/secrets/foo" {
		t.Errorf("SecretURL = %q", got)
	}
}

func TestNewSecretsClientNotSetUp(t *testing.T) {
	useFakeCred(t, "x", nil)
	kv := New("rg", azure.Azure{Location: azure.LocationWestUS2, SubscriptionID: "sub"})
	if _, err := kv.newSecretsClient(); err == nil {
		t.Error("newSecretsClient should fail when vaultURL empty")
	}
}

func TestNewSecretsClientOK(t *testing.T) {
	useFakeCred(t, "tok", nil)
	kv := New("rg", azure.Azure{Location: azure.LocationWestUS2, SubscriptionID: "sub"})
	kv.vaultURL = "https://kv.vault.azure.net"
	if _, err := kv.newSecretsClient(); err != nil {
		t.Errorf("newSecretsClient: %v", err)
	}
}

func TestPutDeleteListSecretNotSetUp(t *testing.T) {
	useFakeCred(t, "x", nil)
	kv := New("rg", azure.Azure{Location: azure.LocationWestUS2, SubscriptionID: "sub"})
	if err := kv.PutSecret(context.Background(), "s", "v", "k"); err == nil {
		t.Error("PutSecret should fail when vault not set up")
	}
	if err := kv.DeleteSecret(context.Background(), "s"); err == nil {
		t.Error("DeleteSecret should fail when vault not set up")
	}
	if _, err := kv.ListSecrets(context.Background(), "prefix"); err == nil {
		t.Error("ListSecrets should fail when vault not set up")
	}
}

// makeResponseError builds an *azcore.ResponseError whose Error() message
// embeds the given response body. retryOnForbiddenByRbac inspects err.Error()
// to detect ForbiddenByRbac in the inner-error JSON, so the body must be set.
func makeResponseError(t *testing.T, status int, body string) *azcore.ResponseError {
	t.Helper()
	return &azcore.ResponseError{
		ErrorCode:  "Forbidden",
		StatusCode: status,
		RawResponse: &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}
}

func TestRetryOnForbiddenByRbac(t *testing.T) {
	const forbiddenBody = `{"error":{"code":"Forbidden","innererror":{"code":"ForbiddenByRbac"}}}`
	const otherForbiddenBody = `{"error":{"code":"Forbidden","message":"some other reason"}}`

	t.Run("success on first try", func(t *testing.T) {
		calls := 0
		err := retryOnForbiddenByRbac(t.Context(), func(context.Context) error {
			calls++
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	t.Run("non-azcore error returned as-is, no retry", func(t *testing.T) {
		want := errors.New("plain error")
		calls := 0
		err := retryOnForbiddenByRbac(t.Context(), func(context.Context) error {
			calls++
			return want
		})
		if !errors.Is(err, want) {
			t.Errorf("err = %v, want %v", err, want)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (should not retry)", calls)
		}
	})

	t.Run("non-403 ResponseError returned as-is, no retry", func(t *testing.T) {
		respErr := makeResponseError(t, 500, forbiddenBody) // body says ForbiddenByRbac, but status is 500
		calls := 0
		err := retryOnForbiddenByRbac(t.Context(), func(context.Context) error {
			calls++
			return respErr
		})
		if !errors.Is(err, respErr) {
			t.Errorf("err = %v, want %v", err, respErr)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (non-403 should not retry)", calls)
		}
	})

	t.Run("403 without ForbiddenByRbac returned as-is, no retry", func(t *testing.T) {
		respErr := makeResponseError(t, 403, otherForbiddenBody)
		calls := 0
		err := retryOnForbiddenByRbac(t.Context(), func(context.Context) error {
			calls++
			return respErr
		})
		if !errors.Is(err, respErr) {
			t.Errorf("err = %v, want %v", err, respErr)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (403 without ForbiddenByRbac should not retry)", calls)
		}
	})

	t.Run("retries 403 ForbiddenByRbac then succeeds", func(t *testing.T) {
		calls := 0
		err := retryOnForbiddenByRbac(t.Context(), func(context.Context) error {
			calls++
			if calls == 1 {
				return makeResponseError(t, 403, forbiddenBody)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("calls = %d, want 2 (one failure then success)", calls)
		}
	})

	t.Run("context cancellation aborts retry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		err := retryOnForbiddenByRbac(ctx, func(context.Context) error {
			calls++
			cancel() // cancel before the function sleeps
			return makeResponseError(t, 403, forbiddenBody)
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1 (cancel should stop retries)", calls)
		}
	})
}

func TestObjectIDFromJWT(t *testing.T) {
	// Build a fake JWT with {"oid":"test-oid-value"} payload.
	payload := `{"sub":"x","oid":"test-oid-value","aud":"y"}`
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	token := "header." + encoded + ".signature"
	if got := objectIDFromJWT(token); got != "test-oid-value" {
		t.Errorf("objectIDFromJWT = %q, want test-oid-value", got)
	}

	// Missing oid claim.
	noOID := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`))
	if got := objectIDFromJWT("h." + noOID + ".s"); got != "" {
		t.Errorf("objectIDFromJWT without oid = %q, want empty", got)
	}

	// Not a JWT (no '.').
	if got := objectIDFromJWT("not-a-jwt"); got != "" {
		t.Errorf("objectIDFromJWT(bad) = %q, want empty", got)
	}

	// Invalid base64 in payload.
	if got := objectIDFromJWT("h.!!!not-base64!!!.s"); got != "" {
		t.Errorf("objectIDFromJWT(bad base64) = %q, want empty", got)
	}
}
