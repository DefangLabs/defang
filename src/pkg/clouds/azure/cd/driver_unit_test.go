package cd

import (
	"context"
	"errors"
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

func TestNew(t *testing.T) {
	d := New("defang-cd", azure.LocationWestUS2)
	if d == nil {
		t.Fatal("New returned nil")
	}
	if d.Location != azure.LocationWestUS2 {
		t.Errorf("Location = %q, want westus2", d.Location)
	}
	if d.resourceGroupPrefix != "defang-cd" {
		t.Errorf("resourceGroupPrefix = %q", d.resourceGroupPrefix)
	}
	if got := d.ResourceGroupName(); got != "defang-cd-westus2" {
		t.Errorf("ResourceGroupName() = %q, want defang-cd-westus2", got)
	}
}

func TestNewEmptyLocation(t *testing.T) {
	d := New("defang-cd", "")
	if d == nil {
		t.Fatal("New returned nil")
	}
	if got := d.ResourceGroupName(); got != "defang-cd-" {
		t.Errorf("ResourceGroupName() = %q, want defang-cd-", got)
	}
}

func TestSetLocation(t *testing.T) {
	d := New("defang-cd", azure.LocationEastUS)
	if got := d.ResourceGroupName(); got != "defang-cd-eastus" {
		t.Errorf("initial ResourceGroupName = %q", got)
	}
	d.SetLocation(azure.LocationWestUS3)
	if d.Location != azure.LocationWestUS3 {
		t.Errorf("Location not updated: %q", d.Location)
	}
	if got := d.ResourceGroupName(); got != "defang-cd-westus3" {
		t.Errorf("ResourceGroupName after SetLocation = %q, want defang-cd-westus3", got)
	}
}

func TestBlobItem(t *testing.T) {
	b := BlobItem{name: "a/b/c.tar.gz", size: 42}
	if b.Name() != "a/b/c.tar.gz" {
		t.Errorf("Name() = %q", b.Name())
	}
	if b.Size() != 42 {
		t.Errorf("Size() = %d", b.Size())
	}
}

func TestNewResourceGroupClientMissingSubscription(t *testing.T) {
	// Default NewCredsFunc requires AZURE_SUBSCRIPTION_ID.
	orig := azure.NewCredsFunc
	azure.NewCredsFunc = func(a azure.Azure) (azcore.TokenCredential, error) {
		if a.SubscriptionID == "" {
			return nil, errors.New("AZURE_SUBSCRIPTION_ID is not set")
		}
		return fakeCred{tok: "x"}, nil
	}
	t.Cleanup(func() { azure.NewCredsFunc = orig })

	d := New("defang-cd", azure.LocationWestUS2)
	if _, err := d.newResourceGroupClient(); err == nil {
		t.Error("newResourceGroupClient should fail without subscription ID")
	}
}

func TestNewResourceGroupClientOK(t *testing.T) {
	useFakeCred(t, "x", nil)
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	if _, err := d.newResourceGroupClient(); err != nil {
		t.Errorf("newResourceGroupClient: %v", err)
	}
}

func TestCreateResourceGroupCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	if err := d.CreateResourceGroup(context.Background(), "rg"); err == nil {
		t.Error("CreateResourceGroup should surface credential error")
	}
}

func TestTearDownCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	if err := d.TearDown(context.Background()); err == nil {
		t.Error("TearDown should surface credential error")
	}
}

func TestGetStorageAccountFromField(t *testing.T) {
	d := New("defang-cd", azure.LocationWestUS2)
	d.StorageAccount = "myacct"
	got, err := d.getStorageAccount(context.Background(), nil)
	if err != nil {
		t.Fatalf("getStorageAccount: %v", err)
	}
	if got != "myacct" {
		t.Errorf("got = %q, want myacct", got)
	}
}

func TestGetStorageAccountFromEnv(t *testing.T) {
	t.Setenv("AZURE_STORAGE_ACCOUNT", "envacct")
	d := New("defang-cd", azure.LocationWestUS2)
	got, err := d.getStorageAccount(context.Background(), nil)
	if err != nil {
		t.Fatalf("getStorageAccount: %v", err)
	}
	if got != "envacct" {
		t.Errorf("got = %q, want envacct", got)
	}
}

func TestSetUpStorageAccountIdempotent(t *testing.T) {
	d := New("defang-cd", azure.LocationWestUS2)
	d.StorageAccount = "acct"
	d.BlobContainerName = "uploads"
	got, err := d.SetUpStorageAccount(context.Background())
	if err != nil {
		t.Fatalf("SetUpStorageAccount: %v", err)
	}
	if got != "acct" {
		t.Errorf("SetUpStorageAccount returned %q, want acct", got)
	}
}

func TestSetUpStorageAccountCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	if _, err := d.SetUpStorageAccount(context.Background()); err == nil {
		t.Error("SetUpStorageAccount should fail on bad cred")
	}
}

func TestSetUpResourceGroupCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	if err := d.SetUpResourceGroup(context.Background()); err == nil {
		t.Error("SetUpResourceGroup should fail on bad cred")
	}
}

func TestCreateUploadURLTooLong(t *testing.T) {
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	long := strings.Repeat("x", 65)
	if _, err := d.CreateUploadURL(context.Background(), long); err == nil {
		t.Error("CreateUploadURL should reject names > 64 chars")
	}
}

func TestCreateUploadURLStorageAccountSetupFails(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	// No pre-populated StorageAccount — SetUpStorageAccount will be called and fail.
	if _, err := d.CreateUploadURL(context.Background(), ""); err == nil {
		t.Error("CreateUploadURL should fail when setup fails")
	}
}

func TestCreateUploadURLWithStorageKeyEnv(t *testing.T) {
	// With AZURE_STORAGE_KEY set and StorageAccount pre-populated, we skip
	// all ARM calls and produce a SAS URL.
	t.Setenv("AZURE_STORAGE_KEY", "dGVzdC1rZXk=") // base64-encoded fake key
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "acct"
	d.BlobContainerName = "uploads"

	got, err := d.CreateUploadURL(context.Background(), "myblob")
	if err != nil {
		t.Fatalf("CreateUploadURL: %v", err)
	}
	if got == "" {
		t.Error("CreateUploadURL returned empty URL")
	}
	if !strings.Contains(got, "acct.blob.core.windows.net") || !strings.Contains(got, "myblob") {
		t.Errorf("URL %q does not look like a SAS URL", got)
	}
}

func TestCreateUploadURLSanitizesSlash(t *testing.T) {
	t.Setenv("AZURE_STORAGE_KEY", "dGVzdC1rZXk=")
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "acct"
	d.BlobContainerName = "uploads"

	got, err := d.CreateUploadURL(context.Background(), "sha256:abc/def")
	if err != nil {
		t.Fatalf("CreateUploadURL: %v", err)
	}
	// Slash in the digest should be replaced with underscore so it's a safe
	// blob name.
	if !strings.Contains(got, "sha256%3Aabc_def") && !strings.Contains(got, "sha256:abc_def") {
		t.Errorf("URL %q did not sanitize slash", got)
	}
}

func TestNewSharedKeyCredentialFromEnv(t *testing.T) {
	t.Setenv("AZURE_STORAGE_KEY", "dGVzdC1rZXk=")
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "myacct"
	cred, err := d.newSharedKeyCredential(context.Background())
	if err != nil {
		t.Fatalf("newSharedKeyCredential: %v", err)
	}
	if cred == nil {
		t.Error("cred should not be nil")
	}
}

func TestNewSharedKeyCredentialCredError(t *testing.T) {
	// No AZURE_STORAGE_KEY and the ARM path fails.
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "acct"
	if _, err := d.newSharedKeyCredential(context.Background()); err == nil {
		t.Error("newSharedKeyCredential should fail without key")
	}
}

func TestNewBlobContainerClientFromEnv(t *testing.T) {
	t.Setenv("AZURE_STORAGE_KEY", "dGVzdC1rZXk=")
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "myacct"
	d.BlobContainerName = "uploads"
	client, err := d.newBlobContainerClient(context.Background())
	if err != nil {
		t.Fatalf("newBlobContainerClient: %v", err)
	}
	if client == nil {
		t.Error("client should not be nil")
	}
}

func TestIterateBlobsCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "acct"
	d.BlobContainerName = "uploads"
	if _, err := d.IterateBlobs(context.Background(), "x"); err == nil {
		t.Error("IterateBlobs should fail without key")
	}
}

func TestDownloadBlobCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "acct"
	d.BlobContainerName = "uploads"
	if _, err := d.DownloadBlob(context.Background(), "blob"); err == nil {
		t.Error("DownloadBlob should fail without key")
	}
}

func TestCreateUploadURLGenerateBlobName(t *testing.T) {
	// With empty blobName the driver generates a UUID.
	t.Setenv("AZURE_STORAGE_KEY", "dGVzdC1rZXk=")
	d := New("defang-cd", azure.LocationWestUS2)
	d.SubscriptionID = "sub"
	d.StorageAccount = "acct"
	d.BlobContainerName = "uploads"
	url, err := d.CreateUploadURL(context.Background(), "")
	if err != nil {
		t.Fatalf("CreateUploadURL: %v", err)
	}
	if url == "" {
		t.Error("URL should be non-empty")
	}
}
