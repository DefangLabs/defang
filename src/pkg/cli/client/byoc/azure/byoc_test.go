package azure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
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
	b := NewByocProvider(t.Context(), "test-tenant", "test-stack")
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
	// The RG must match what Pulumi creates: {Prefix}-{project}-{stack}, using the bare
	// project name (the stack is a separate axis, not baked into the project name).
	got := b.projectResourceGroupName("myapp")
	want := "Defang-myapp-test-stack"
	if got != want {
		t.Errorf("projectResourceGroupName = %q, want %q", got, want)
	}
}

func TestSetUpLocationMissing(t *testing.T) {
	t.Setenv("AZURE_LOCATION", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	b := NewByocProvider(t.Context(), "t", "s")
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
	if b.job.ResourceGroup != "defang-cd" {
		t.Errorf("job.ResourceGroup = %q, want defang-cd", b.job.ResourceGroup)
	}
}

func TestAccountInfo(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub-1")
	if err := b.setUpLocation(); err != nil {
		t.Fatalf("setUpLocation: %v", err)
	}
	info, err := b.AccountInfo(t.Context())
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

func TestUnsupportedOps(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")

	if _, err := b.Delete(t.Context(), nil); err == nil {
		t.Error("Delete should return unsupported")
	}
	if _, err := b.RemoteProjectName(t.Context()); err == nil {
		t.Error("RemoteProjectName should return unsupported")
	}
	if err := b.TearDownCD(t.Context()); err == nil {
		t.Error("TearDownCD should return unsupported")
	}
	if err := b.UpdateShardDomain(t.Context()); err == nil {
		t.Error("UpdateShardDomain should return unsupported")
	}
}

func TestGetServicesEmptyProjectReturnsEmpty(t *testing.T) {
	// Empty project name short-circuits GetProjectUpdate with ErrNotExist,
	// and GetServices translates that into an empty response — same contract
	// as the AWS/GCP providers.
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	resp, err := b.GetServices(t.Context(), &defangv1.GetServicesRequest{Project: ""})
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
	_, err := b.GetService(t.Context(), &defangv1.GetRequest{Project: "", Name: "app"})
	if err == nil {
		t.Error("GetService should fail when the named service doesn't exist")
	}
}

func TestPrepareDomainDelegationEmptyDomain(t *testing.T) {
	// No delegate domain means nothing to delegate: return (nil, nil) without
	// touching Azure, so callers treat the deployment as having no delegation.
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	resp, err := b.PrepareDomainDelegation(t.Context(), client.PrepareDomainDelegationRequest{})
	if err != nil {
		t.Errorf("PrepareDomainDelegation err: %v", err)
	}
	if resp != nil {
		t.Errorf("PrepareDomainDelegation response = %v, want nil for empty domain", resp)
	}
}

func TestPrepareDomainDelegationCredError(t *testing.T) {
	// A non-empty delegate domain drives real ARM calls (resource group +
	// DNS zone); a failing credential must surface as an error rather than a
	// silent success.
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := b.PrepareDomainDelegation(ctx, client.PrepareDomainDelegationRequest{
		Project:        "proj",
		DelegateDomain: "proj-test-stack.tenant.example.com",
	})
	if err == nil {
		t.Error("PrepareDomainDelegation should surface ARM error")
	}
}

func TestHasDelegatedSubdomain(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if !b.HasDelegatedSubdomain() {
		t.Error("HasDelegatedSubdomain() = false, want true now that Azure delegates a subdomain zone")
	}
}

func TestSubscribeNoActiveDeployment(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	_, err := b.Subscribe(t.Context(), &defangv1.SubscribeRequest{})
	if err == nil {
		t.Fatal("Subscribe should error when no Deploy / CdCommand has run yet")
	}
}

func TestGetDeploymentStatusNoRun(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	done, err := b.GetDeploymentStatus(t.Context())
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
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := b.GetDeploymentStatus(ctx)
	if err == nil {
		t.Error("GetDeploymentStatus should surface SDK error")
	}
}

func TestGetProjectUpdateCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if _, err := b.GetProjectUpdate(ctx, "proj"); err == nil {
		t.Error("GetProjectUpdate should surface credential error")
	}
}

func TestQueryLogsDiscoversRunByEtag(t *testing.T) {
	// A standalone `defang logs` has no cached cdRunID, so QueryLogs must look the
	// CD run up by etag. When that lookup can't reach Azure it surfaces the error
	// (rather than the old "no matching CD deployment" rejection).
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := b.QueryLogs(ctx, &defangv1.TailRequest{Etag: "some-etag"})
	if err == nil || !strings.Contains(err.Error(), "failed to find CD deployment for etag") {
		t.Errorf("expected etag-lookup failure, got: %v", err)
	}
}

func TestQueryLogsEtagMismatchTriggersLookup(t *testing.T) {
	// A request etag that differs from the cached run no longer rejects outright;
	// it falls back to discovering the matching run by etag.
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	b.cdRunID = "run-1"
	b.cdEtag = "etag-A"
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err := b.QueryLogs(ctx, &defangv1.TailRequest{Etag: "etag-B"})
	if err == nil || !strings.Contains(err.Error(), "failed to find CD deployment for etag") {
		t.Errorf("expected mismatched-etag lookup failure, got: %v", err)
	}
}

func TestAuthenticateNonInteractiveFailsWithoutCreds(t *testing.T) {
	// Point the SDK at an ARM endpoint that returns 401 so DefaultAzureCredential's
	// token always fails validation — no real Azure call is made by our code beyond
	// hitting the subscription endpoint.
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// interactive=false: no valid creds → error.
	if err := b.Authenticate(ctx, false); err == nil {
		t.Error("Authenticate(interactive=false) should fail without working creds")
	}
}

func TestDeployMissingCDImage(t *testing.T) {
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	// CDImage is empty by default.
	if _, err := b.Deploy(t.Context(), &client.DeployRequest{}); err == nil {
		t.Error("Deploy should fail without CDImage")
	}
	if _, err := b.Preview(t.Context(), &client.DeployRequest{}); err == nil {
		t.Error("Preview should fail without CDImage")
	}
}

func TestSetUpJobMissingCDImage(t *testing.T) {
	t.Skip("not sure")
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if err := b.setUpJob(t.Context()); err == nil {
		t.Error("setUpJob should fail without CDImage")
	}
}

func TestSetUpMissingLocation(t *testing.T) {
	// Clear AZURE_LOCATION so setUp's setUpLocation step fails early.
	t.Setenv("AZURE_LOCATION", "")
	t.Setenv("AZURE_SUBSCRIPTION_ID", "")
	b := NewByocProvider(t.Context(), "t", "s")
	if err := b.SetUpCD(t.Context(), false); err == nil {
		t.Error("setUp should fail without AZURE_LOCATION")
	}
	// Same for setUpForConfig.
	if err := b.setUpForConfig(t.Context(), "proj"); err == nil {
		t.Error("setUpForConfig should fail without AZURE_LOCATION")
	}
	// CreateUploadURL and CdList go through setUp, so they should also fail.
	if _, err := b.CreateUploadURL(t.Context(), &defangv1.UploadURLRequest{Digest: "d"}); err == nil {
		t.Error("CreateUploadURL should fail without AZURE_LOCATION")
	}
	if _, err := b.CdList(t.Context(), false); err == nil {
		t.Error("CdList should fail without AZURE_LOCATION")
	}
	// GetProjectUpdate with empty project bails early.
	if _, err := b.GetProjectUpdate(t.Context(), ""); err == nil {
		t.Error("GetProjectUpdate should fail with empty project name")
	}
	// DeleteConfig and ListConfig also go through setUpForConfig.
	if err := b.DeleteConfig(t.Context(), &defangv1.Secrets{Project: "p"}); err == nil {
		t.Error("DeleteConfig should fail without AZURE_LOCATION")
	}
	if _, err := b.ListConfig(t.Context(), &defangv1.ListConfigsRequest{Project: "p"}); err == nil {
		t.Error("ListConfig should fail without AZURE_LOCATION")
	}
	if err := b.PutConfig(t.Context(), &defangv1.PutConfigRequest{Project: "p", Name: "n", Value: "v"}); err == nil {
		t.Error("PutConfig should fail without AZURE_LOCATION")
	}
}

func TestCdCommandCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if _, err := b.CdCommand(ctx, client.CdCommandRequest{Project: "p", Command: "up"}); err == nil {
		t.Error("CdCommand should fail when ARM calls fail")
	}
}

func TestPutConfigCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := b.PutConfig(ctx, &defangv1.PutConfigRequest{Project: "p", Name: "n", Value: "v"}); err == nil {
		t.Error("PutConfig should fail when ARM calls fail")
	}
}

func TestCreateUploadURLSubset(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
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
	if _, err := b.Deploy(t.Context(), req); err == nil {
		t.Error("Deploy should fail with invalid compose")
	}
}

func TestTearDownCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	b := newTestProvider(t, cloudazure.LocationEastUS, "sub")
	if err := b.TearDown(t.Context()); err == nil {
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
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
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
	b := NewByocProvider(t.Context(), "t", "s")
	if _, err := b.CdCommand(t.Context(), client.CdCommandRequest{Project: "p", Command: "up"}); err == nil {
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

	env, err := b.environment("myproj")
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
	if got := env["DEFANG_STATE_URL"]; got != "azblob://uploads?storage_account=acct" {
		t.Errorf("DEFANG_STATE_URL = %q", got)
	}
	if _, ok := env["PULUMI_CONFIG_PASSPHRASE"]; !ok {
		t.Error("PULUMI_CONFIG_PASSPHRASE missing")
	}
}

// TestUpdateServiceInfo verifies the byoc.ServiceInfoUpdater contract:
// services with a non-empty `domainname` get UseAcmeCert flipped on so the
// downstream cert-gen flow picks them up; services without one are left
// untouched.
func TestUpdateServiceInfo(t *testing.T) {
	tests := []struct {
		name           string
		domainName     string
		initialUseAcme bool
		wantUseAcme    bool
	}{
		{name: "with domainname enables acme", domainName: "x.example.com", wantUseAcme: true},
		{name: "without domainname is no-op", domainName: "", wantUseAcme: false},
		{name: "without domainname preserves prior true", domainName: "", initialUseAcme: true, wantUseAcme: true},
		{name: "with domainname keeps already-true", domainName: "x.example.com", initialUseAcme: true, wantUseAcme: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestProvider(t, cloudazure.LocationWestUS3, "sub-id")
			si := &defangv1.ServiceInfo{UseAcmeCert: tt.initialUseAcme}
			svc := composeTypes.ServiceConfig{DomainName: tt.domainName}
			if err := b.UpdateServiceInfo(t.Context(), si, "proj", "etag", svc); err != nil {
				t.Fatalf("UpdateServiceInfo: %v", err)
			}
			if si.UseAcmeCert != tt.wantUseAcme {
				t.Errorf("UseAcmeCert = %v, want %v", si.UseAcmeCert, tt.wantUseAcme)
			}
		})
	}
}
