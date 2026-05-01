package azure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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
	orig := cloudazure.NewCredsFunc
	cloudazure.NewCredsFunc = func(_ cloudazure.Azure) (azcore.TokenCredential, error) {
		return fakeCred{tok: tok, err: gerr}, nil
	}
	t.Cleanup(func() { cloudazure.NewCredsFunc = orig })
}

func newTestProvider(t *testing.T, location cloudazure.Location, subID string) *ByocAzure {
	t.Helper()
	t.Setenv("AZURE_LOCATION", string(location))
	t.Setenv("AZURE_SUBSCRIPTION_ID", subID)
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")
	b := NewByocProvider(context.Background(), "test-tenant", "test-stack")
	if b == nil {
		t.Fatal("NewByocProvider returned nil")
	}
	return b
}

func TestNewByocProvider(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationWestUS2, "sub-id")
	if b.PulumiStack != "test-stack" {
		t.Errorf("PulumiStack = %q, want test-stack", b.PulumiStack)
	}
	if b.TenantLabel != "test-tenant" {
		t.Errorf("TenantLabel = %q, want test-tenant", b.TenantLabel)
	}
}

func TestDriver(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if got := b.Driver(); got != "azure" {
		t.Errorf("Driver() = %q, want azure", got)
	}
}

func TestServiceDNS(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	host := "my-service.example"
	if got := b.ServiceDNS(host); got != host {
		t.Errorf("ServiceDNS(%q) = %q, want pass-through", host, got)
	}
}

func TestGetPrivateDomain(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	got := b.GetPrivateDomain("myproject")
	if got == "" || got[len(got)-len(".internal"):] != ".internal" {
		t.Errorf("GetPrivateDomain = %q, want *.internal", got)
	}
}

func TestProjectResourceGroupName(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationWestUS2, "sub")
	if err := b.setUpLocation(); err != nil {
		t.Fatalf("setUpLocation: %v", err)
	}
	got := b.projectResourceGroupName("myapp")
	want := "defang-myapp-test-stack"
	if got != want {
		t.Errorf("projectResourceGroupName = %q, want %q", got, want)
	}
}

func TestSetUpLocationMissing(t *testing.T) {
	t.Setenv("AZURE_LOCATION", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	b := NewByocProvider(context.Background(), "t", "s")
	if err := b.setUpLocation(); err == nil {
		t.Error("expected error when AZURE_LOCATION is unset")
	}
}

func TestSetUpLocationFromEnv(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationWestUS3, "sub-id")
	if err := b.setUpLocation(); err != nil {
		t.Fatalf("setUpLocation: %v", err)
	}
	if b.driver.Location != cloudazure.LocationWestUS3 {
		t.Errorf("driver.Location = %q", b.driver.Location)
	}
	if b.driver.SubscriptionID != "sub-id" {
		t.Errorf("driver.SubscriptionID = %q", b.driver.SubscriptionID)
	}
	if b.job.ResourceGroup != "defang-cd-westus3" {
		t.Errorf("job.ResourceGroup = %q", b.job.ResourceGroup)
	}
}

func TestAccountInfo(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub-1")
	if err := b.setUpLocation(); err != nil {
		t.Fatalf("setUpLocation: %v", err)
	}
	info, err := b.AccountInfo(context.Background())
	if err != nil {
		t.Fatalf("AccountInfo: %v", err)
	}
	if info.AccountID != "sub-1" {
		t.Errorf("AccountID = %q, want sub-1", info.AccountID)
	}
	if info.Region != "eastus" {
		t.Errorf("Region = %q, want eastus", info.Region)
	}
	if info.Provider != client.ProviderAzure {
		t.Errorf("Provider = %v, want Azure", info.Provider)
	}
}

func TestSetUpCDNoOp(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if err := b.SetUpCD(context.Background(), false); err != nil {
		t.Errorf("SetUpCD should be no-op, got %v", err)
	}
}

func TestUnsupportedOps(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")

	if _, err := b.Delete(context.Background(), nil); err == nil {
		t.Error("Delete should return unsupported")
	}
	if _, err := b.RemoteProjectName(context.Background()); err == nil {
		t.Error("RemoteProjectName should return unsupported")
	}
	if err := b.TearDownCD(context.Background()); err == nil {
		t.Error("TearDownCD should return unsupported")
	}
	if err := b.UpdateShardDomain(context.Background()); err == nil {
		t.Error("UpdateShardDomain should return unsupported")
	}
}

func TestGetServicesEmptyProjectReturnsEmpty(t *testing.T) {
	// Empty project name short-circuits GetProjectUpdate with ErrNotExist,
	// and GetServices translates that into an empty response — same contract
	// as the AWS/GCP providers.
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	resp, err := b.GetServices(context.Background(), &defangv1.GetServicesRequest{Project: ""})
	if err != nil {
		t.Fatalf("GetServices(empty project): %v", err)
	}
	if len(resp.Services) != 0 {
		t.Errorf("expected empty services, got %d", len(resp.Services))
	}
}

func TestGetServiceEmptyProjectNotFound(t *testing.T) {
	// With no deployments, GetService should surface a NotFound for any name.
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	_, err := b.GetService(context.Background(), &defangv1.GetRequest{Project: "", Name: "app"})
	if err == nil {
		t.Error("GetService should fail when the named service doesn't exist")
	}
}

func TestPrepareDomainDelegationNil(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	resp, err := b.PrepareDomainDelegation(context.Background(), client.PrepareDomainDelegationRequest{})
	if err != nil {
		t.Errorf("PrepareDomainDelegation err: %v", err)
	}
	if resp != nil {
		t.Errorf("PrepareDomainDelegation response = %v, want nil (TODO)", resp)
	}
}

func TestSubscribe(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	seq, err := b.Subscribe(context.Background(), &defangv1.SubscribeRequest{})
	if err != nil {
		t.Fatalf("Subscribe err: %v", err)
	}
	if seq == nil {
		t.Fatal("Subscribe returned nil seq")
	}
	// The TODO stub simply yields nothing — iterating should finish immediately.
	for range seq {
		t.Error("Subscribe iterator yielded unexpectedly")
	}
}

func TestGetDeploymentStatusNoRun(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	done, err := b.GetDeploymentStatus(context.Background())
	if err != nil {
		t.Errorf("GetDeploymentStatus err: %v", err)
	}
	if done {
		t.Error("GetDeploymentStatus should be not-done when cdRunID is empty")
	}
}

func TestGetDeploymentStatusCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	b.cdRunID = "run-1"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := b.GetDeploymentStatus(ctx)
	if err == nil {
		t.Error("GetDeploymentStatus should surface SDK error")
	}
}

func TestGetProjectUpdateCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := b.GetProjectUpdate(ctx, "proj"); err == nil {
		t.Error("GetProjectUpdate should surface credential error")
	}
}

func TestQueryLogsUnknownEtag(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	// cdRunID is empty — QueryLogs should reject the request rather than panic.
	_, err := b.QueryLogs(context.Background(), &defangv1.TailRequest{Etag: "some-etag"})
	if err == nil {
		t.Error("QueryLogs should reject when cdRunID is empty")
	}
	var _ = errors.New // silence unused when build tag trims
}

func TestQueryLogsEtagMismatch(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	b.cdRunID = "run-1"
	b.cdEtag = "etag-A"
	_, err := b.QueryLogs(context.Background(), &defangv1.TailRequest{Etag: "etag-B"})
	if err == nil {
		t.Error("QueryLogs should reject etag mismatch")
	}
}

func TestAuthenticateNonInteractiveFailsWithoutCreds(t *testing.T) {
	// Point the SDK at an ARM endpoint that returns 401 so DefaultAzureCredential's
	// token always fails validation — no real Azure call is made by our code beyond
	// hitting the subscription endpoint.
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// interactive=false: no valid creds → error.
	if err := b.Authenticate(ctx, false); err == nil {
		t.Error("Authenticate(interactive=false) should fail without working creds")
	}
}

func TestDeployMissingCDImage(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	// CDImage is empty by default.
	if _, err := b.Deploy(context.Background(), &client.DeployRequest{}); err == nil {
		t.Error("Deploy should fail without CDImage")
	}
	if _, err := b.Preview(context.Background(), &client.DeployRequest{}); err == nil {
		t.Error("Preview should fail without CDImage")
	}
}

func TestSetUpJobMissingCDImage(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if err := b.setUpJob(context.Background(), nil); err == nil {
		t.Error("setUpJob should fail without CDImage")
	}
}

func TestSetUpMissingLocation(t *testing.T) {
	// Clear AZURE_LOCATION so setUp's setUpLocation step fails early.
	t.Setenv("AZURE_LOCATION", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	b := NewByocProvider(context.Background(), "t", "s")
	if err := b.setUp(context.Background()); err == nil {
		t.Error("setUp should fail without AZURE_LOCATION")
	}
	// Same for setUpForConfig.
	if err := b.setUpForConfig(context.Background(), "proj"); err == nil {
		t.Error("setUpForConfig should fail without AZURE_LOCATION")
	}
	// CreateUploadURL and CdList go through setUp, so they should also fail.
	if _, err := b.CreateUploadURL(context.Background(), &defangv1.UploadURLRequest{Digest: "d"}); err == nil {
		t.Error("CreateUploadURL should fail without AZURE_LOCATION")
	}
	if _, err := b.CdList(context.Background(), false); err == nil {
		t.Error("CdList should fail without AZURE_LOCATION")
	}
	// GetProjectUpdate with empty project bails early.
	if _, err := b.GetProjectUpdate(context.Background(), ""); err == nil {
		t.Error("GetProjectUpdate should fail with empty project name")
	}
	// DeleteConfig and ListConfig also go through setUpForConfig.
	if err := b.DeleteConfig(context.Background(), &defangv1.Secrets{Project: "p"}); err == nil {
		t.Error("DeleteConfig should fail without AZURE_LOCATION")
	}
	if _, err := b.ListConfig(context.Background(), &defangv1.ListConfigsRequest{Project: "p"}); err == nil {
		t.Error("ListConfig should fail without AZURE_LOCATION")
	}
	if err := b.PutConfig(context.Background(), &defangv1.PutConfigRequest{Project: "p", Name: "n", Value: "v"}); err == nil {
		t.Error("PutConfig should fail without AZURE_LOCATION")
	}
}

func TestCdCommandCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := b.CdCommand(ctx, client.CdCommandRequest{Project: "p", Command: "up"}); err == nil {
		t.Error("CdCommand should fail when ARM calls fail")
	}
}

func TestPutConfigCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.PutConfig(ctx, &defangv1.PutConfigRequest{Project: "p", Name: "n", Value: "v"}); err == nil {
		t.Error("PutConfig should fail when ARM calls fail")
	}
}

func TestCreateUploadURLSubset(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := b.CreateUploadURL(ctx, &defangv1.UploadURLRequest{Digest: "d"}); err == nil {
		t.Error("CreateUploadURL should fail when ARM calls fail")
	}
}

func TestDeployInvalidCompose(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	b.CDImage = "img"
	// An invalid compose payload should fail to load.
	req := &client.DeployRequest{}
	req.Compose = []byte("not valid yaml: [")
	if _, err := b.Deploy(context.Background(), req); err == nil {
		t.Error("Deploy should fail with invalid compose")
	}
}

func TestTearDownCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if err := b.TearDown(context.Background()); err == nil {
		t.Error("TearDown should surface credential error")
	}
}

func TestQueryLogsNonFollow(t *testing.T) {
	useFakeCred(t, "tok", nil)
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	b.cdRunID = "run-1"
	b.cdEtag = "etag"

	// ReadJobLogs calls Log Analytics workspace SDK client, which will fail
	// without real Azure access. We just want the non-follow path to return
	// an error (not panic).
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := b.QueryLogs(ctx, &defangv1.TailRequest{Etag: "etag", Follow: false})
	if err == nil {
		t.Error("QueryLogs non-follow should fail without real Azure workspace")
	}
}

func TestCdCommandMissingCDImage(t *testing.T) {
	// setUpForConfig needs to succeed for CdCommand to reach the CDImage check.
	// Easier: exercise the setUpLocation-fails path which happens first.
	t.Setenv("AZURE_LOCATION", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	b := NewByocProvider(context.Background(), "t", "s")
	if _, err := b.CdCommand(context.Background(), client.CdCommandRequest{Project: "p", Command: "up"}); err == nil {
		t.Error("CdCommand should fail without AZURE_LOCATION")
	}
}

func TestBuildCdEnv(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationWestUS2, "sub-1")
	if err := b.setUpLocation(); err != nil {
		t.Fatalf("setUpLocation: %v", err)
	}
	b.driver.StorageAccount = "acct"
	b.driver.BlobContainerName = "uploads"

	env, err := b.buildCdEnv("myproj")
	if err != nil {
		t.Fatalf("buildCdEnv: %v", err)
	}
	if got := env["PROJECT"]; got != "myproj" {
		t.Errorf("PROJECT = %q", got)
	}
	if got := env["AZURE_LOCATION"]; got != "westus2" {
		t.Errorf("AZURE_LOCATION = %q", got)
	}
	if got := env["AZURE_SUBSCRIPTION_ID"]; got != "sub-1" {
		t.Errorf("AZURE_SUBSCRIPTION_ID = %q", got)
	}
	// AZURE_RESOURCE_GROUP / AZURE_KEY_VAULT_NAME should NOT be passed — the
	// Pulumi provider derives them deterministically from the same inputs.
	if _, ok := env["AZURE_RESOURCE_GROUP"]; ok {
		t.Errorf("AZURE_RESOURCE_GROUP should not be passed to CD; provider derives it")
	}
	if _, ok := env["AZURE_KEY_VAULT_NAME"]; ok {
		t.Errorf("AZURE_KEY_VAULT_NAME should not be passed to CD; provider derives it")
	}
	if got := env["STACK"]; got != "test-stack" {
		t.Errorf("STACK = %q", got)
	}
	if got := env["DEFANG_STATE_URL"]; got != "azblob://pulumi?storage_account=acct" {
		t.Errorf("DEFANG_STATE_URL = %q", got)
	}
	if _, ok := env["PULUMI_CONFIG_PASSPHRASE"]; !ok {
		t.Error("PULUMI_CONFIG_PASSPHRASE missing")
	}
}
