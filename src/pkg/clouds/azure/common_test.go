package azure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// fakeCredential is a stub TokenCredential used to bypass azidentity in tests.
type fakeCredential struct {
	token string
	err   error
}

func (f fakeCredential) GetToken(ctx context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	if f.err != nil {
		return azcore.AccessToken{}, f.err
	}
	return azcore.AccessToken{Token: f.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// useFakeCred swaps in a fake credential for the duration of the test.
func useFakeCred(t *testing.T, tok string, gerr error) {
	t.Helper()
	orig := NewCredsFunc
	NewCredsFunc = func(_ Azure) (azcore.TokenCredential, error) {
		return fakeCredential{token: tok, err: gerr}, nil
	}
	t.Cleanup(func() { NewCredsFunc = orig })
}

// useTestEndpoint swaps ManagementEndpoint to the httptest.Server URL.
func useTestEndpoint(t *testing.T, url string) {
	t.Helper()
	orig := ManagementEndpoint
	ManagementEndpoint = url
	t.Cleanup(func() { ManagementEndpoint = orig })
}

func TestArmToken(t *testing.T) {
	useFakeCred(t, "my-arm-token", nil)
	a := Azure{SubscriptionID: "sub"}
	got, err := a.ArmToken(context.Background())
	if err != nil {
		t.Fatalf("ArmToken: %v", err)
	}
	if got != "my-arm-token" {
		t.Errorf("ArmToken = %q", got)
	}
}

func TestArmTokenCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("auth failed"))
	a := Azure{SubscriptionID: "sub"}
	if _, err := a.ArmToken(context.Background()); err == nil {
		t.Error("ArmToken should propagate credential error")
	}
}

func TestNewCredsMissingSub(t *testing.T) {
	// Reset to the default implementation so we hit the AZURE_SUBSCRIPTION_ID check.
	orig := NewCredsFunc
	NewCredsFunc = func(a Azure) (azcore.TokenCredential, error) {
		if a.SubscriptionID == "" {
			return nil, errors.New("AZURE_SUBSCRIPTION_ID is not set")
		}
		return fakeCredential{token: "x"}, nil
	}
	t.Cleanup(func() { NewCredsFunc = orig })

	a := Azure{}
	if _, err := a.NewCreds(); err == nil {
		t.Error("NewCreds should fail without subscription ID")
	}
}

func TestFetchLogStreamAuthToken(t *testing.T) {
	var gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		if !strings.Contains(r.URL.Path, "Microsoft.App/jobs/defang-cd/getAuthToken") {
			t.Errorf("path = %q, want contains Microsoft.App/jobs/defang-cd/getAuthToken", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"properties": {"token": "stream-token-abc"}}`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm-token", nil)
	useTestEndpoint(t, srv.URL)

	a := Azure{SubscriptionID: "sub"}
	got, err := a.FetchLogStreamAuthToken(context.Background(), "rg", "Microsoft.App/jobs/defang-cd", "2024-02-02-preview")
	if err != nil {
		t.Fatalf("FetchLogStreamAuthToken: %v", err)
	}
	if got != "stream-token-abc" {
		t.Errorf("token = %q, want stream-token-abc", got)
	}
	if gotAuth != "Bearer arm-token" {
		t.Errorf("Authorization header = %q", gotAuth)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
}

func TestFetchLogStreamAuthTokenNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	useFakeCred(t, "arm-token", nil)
	useTestEndpoint(t, srv.URL)

	a := Azure{SubscriptionID: "sub"}
	_, err := a.FetchLogStreamAuthToken(context.Background(), "rg", "Microsoft.App/jobs/x", "2024-02-02-preview")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestFetchLogStreamAuthTokenArmTokenError(t *testing.T) {
	useFakeCred(t, "", errors.New("arm denied"))
	a := Azure{SubscriptionID: "sub"}
	if _, err := a.FetchLogStreamAuthToken(context.Background(), "rg", "x", "v"); err == nil {
		t.Error("expected error when ArmToken fails")
	}
}

func TestFetchLogStreamAuthTokenBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm-token", nil)
	useTestEndpoint(t, srv.URL)

	a := Azure{SubscriptionID: "sub"}
	_, err := a.FetchLogStreamAuthToken(context.Background(), "rg", "x", "v")
	if err == nil {
		t.Error("expected decode error")
	}
}
