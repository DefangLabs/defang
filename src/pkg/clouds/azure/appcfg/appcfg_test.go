package appcfg

import (
	"context"
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

func TestStoreName(t *testing.T) {
	name := StoreName("my-rg", "sub-id")
	if !strings.HasPrefix(name, "my-rg-") {
		t.Errorf("StoreName = %q, want my-rg- prefix", name)
	}
	if len(name) > 50 {
		t.Errorf("StoreName %q exceeds 50 chars", name)
	}
	// Deterministic.
	if StoreName("my-rg", "sub-id") != name {
		t.Error("StoreName is not deterministic")
	}
	// Different inputs produce different names.
	if StoreName("other-rg", "sub-id") == name {
		t.Error("StoreName collision for different resource group")
	}
	if StoreName("my-rg", "other-sub") == name {
		t.Error("StoreName collision for different subscription")
	}
}

func TestStoreNameLongResourceGroup(t *testing.T) {
	// Long resource group must be truncated so the full name fits in 50 chars.
	long := strings.Repeat("a", 80)
	name := StoreName(long, "sub")
	if len(name) > 50 {
		t.Errorf("StoreName %q exceeds 50 chars for long rg", name)
	}
	if !strings.Contains(name, "-") {
		t.Errorf("StoreName %q missing suffix separator", name)
	}
}

func TestNew(t *testing.T) {
	cfg := New("rg", azure.LocationEastUS, "sub-id")
	if cfg == nil {
		t.Fatal("New returned nil")
	}
	if cfg.SubscriptionID != "sub-id" {
		t.Errorf("SubscriptionID = %q, want sub-id", cfg.SubscriptionID)
	}
	if cfg.Location != azure.LocationEastUS {
		t.Errorf("Location = %q, want eastus", cfg.Location)
	}
	if cfg.resourceGroupName != "rg" {
		t.Errorf("resourceGroupName = %q, want rg", cfg.resourceGroupName)
	}
}

func TestNewDataClientNotSetUp(t *testing.T) {
	cfg := New("rg", azure.LocationEastUS, "sub")
	if _, err := cfg.newDataClient(); err == nil {
		t.Error("newDataClient expected error when connection string is empty")
	}
}

func TestPutDeleteListSettingNotSetUp(t *testing.T) {
	useFakeCred(t, "x", nil)
	cfg := New("rg", azure.LocationEastUS, "sub")
	if err := cfg.PutSetting(context.Background(), "k", "v"); err == nil {
		t.Error("PutSetting should fail before SetUp")
	}
	if err := cfg.DeleteSetting(context.Background(), "k"); err == nil {
		t.Error("DeleteSetting should fail before SetUp")
	}
	if _, err := cfg.ListSettings(context.Background(), "prefix"); err == nil {
		t.Error("ListSettings should fail before SetUp")
	}
}
