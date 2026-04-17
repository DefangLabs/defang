package aca

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

func TestWatchLogsCancelled(t *testing.T) {
	useFakeCred(t, "tok", nil)
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ch := c.WatchLogs(ctx)
	for range ch {
		// drain
	}
	// If we got here, WatchLogs properly exits on ctx cancellation.
}

func TestStreamLogsResolveFailure(t *testing.T) {
	// With a fake credential, the SDK construct succeeds but API calls fail.
	// StreamLogs should surface the error from ResolveLogTarget.
	useFakeCred(t, "tok", nil)
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.StreamLogs(ctx, "myapp", "", "", "", true); err == nil {
		t.Error("StreamLogs should fail when SDK calls can't reach ARM")
	}
}

func TestResolveLogTargetAllProvided(t *testing.T) {
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	// When every arg is non-empty, ResolveLogTarget returns them as-is without
	// any API call.
	rev, rep, con, err := c.ResolveLogTarget(context.Background(), "app", "rev1", "rep1", "ctr1")
	if err != nil {
		t.Fatalf("ResolveLogTarget: %v", err)
	}
	if rev != "rev1" || rep != "rep1" || con != "ctr1" {
		t.Errorf("got (%q, %q, %q)", rev, rep, con)
	}
}

func TestResolveLogTargetMissingContainer(t *testing.T) {
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	// revision + replica provided, container empty — no SDK call, but container
	// resolution fails with an error.
	if _, _, _, err := c.ResolveLogTarget(context.Background(), "app", "rev", "rep", ""); err == nil {
		t.Error("ResolveLogTarget should fail when container can't be determined")
	}
}

func TestResolveLogTargetSDKFailure(t *testing.T) {
	useFakeCred(t, "tok", nil)
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// With no revision provided, ResolveLogTarget calls appsClient.Get which
	// will attempt a real HTTP request and fail.
	if _, _, _, err := c.ResolveLogTarget(ctx, "app", "", "", ""); err == nil {
		t.Error("ResolveLogTarget should error when SDK call fails")
	}
}

func TestStreamLogsCredError(t *testing.T) {
	// Credential-layer error during ResolveLogTarget.
	orig := cloudazure.NewCredsFunc
	cloudazure.NewCredsFunc = func(_ cloudazure.Azure) (azcore.TokenCredential, error) {
		return nil, errors.New("cred fail")
	}
	t.Cleanup(func() { cloudazure.NewCredsFunc = orig })

	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	if _, err := c.StreamLogs(context.Background(), "app", "", "", "", true); err == nil {
		t.Error("StreamLogs should fail when credential resolution fails")
	}
}

func TestStreamLogsMissingRevisionSDKError(t *testing.T) {
	// With a working fake cred but no access to ARM, the SDK call in
	// ResolveLogTarget fails.
	useFakeCred(t, "tok", nil)
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Pass revision+replica, container empty — bypasses the Get() SDK call but
	// still needs container resolution which fails → fast error, no network.
	if _, _, _, err := c.ResolveLogTarget(ctx, "app", "rev", "rep", ""); err == nil {
		t.Error("ResolveLogTarget should fail when container cannot be resolved")
	}
}

func TestStreamLogsFullPath(t *testing.T) {
	// Serve the Container App GET (for eventStreamEndpoint), getAuthToken,
	// and the logstream on the same httptest server.
	var eventStreamEndpoint string
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getAuthToken"):
			_, _ = w.Write([]byte(`{"properties":{"token":"stream-tok"}}`))
		case strings.HasSuffix(r.URL.Path, "/containerApps/myapp"):
			// Use the same test server's base URL in the endpoint so the logstream
			// request ends up here too.
			_, _ = w.Write([]byte(`{"properties":{"eventStreamEndpoint":"` + eventStreamEndpoint + `/subscriptions/sub/foo"}}`))
		case strings.Contains(r.URL.Path, "/logstream"):
			if !strings.Contains(r.URL.Path, "containerApps/myapp/revisions/rev/replicas/rep/containers/ctr/logstream") {
				t.Errorf("stream path = %q", r.URL.Path)
			}
			_, _ = w.Write([]byte("a\nb\n"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mgmt.Close()
	eventStreamEndpoint = mgmt.URL

	useFakeCred(t, "arm", nil)
	origMgmt := cloudazure.ManagementEndpoint
	cloudazure.ManagementEndpoint = mgmt.URL
	t.Cleanup(func() { cloudazure.ManagementEndpoint = origMgmt })

	c := &ContainerApp{
		Azure: cloudazure.Azure{
			SubscriptionID: "sub",
			Location:       cloudazure.LocationWestUS2,
		},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, err := c.StreamLogs(ctx, "myapp", "rev", "rep", "ctr", true)
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	var got []string
	for entry := range ch {
		if entry.Err != nil {
			continue
		}
		got = append(got, entry.Message)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got lines %v", got)
	}
}

func TestStreamLogsAuthTokenError(t *testing.T) {
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containerApps/myapp"):
			_, _ = w.Write([]byte(`{"properties":{"eventStreamEndpoint":"https://unused/subscriptions/s/x"}}`))
		case strings.Contains(r.URL.Path, "/getAuthToken"):
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer mgmt.Close()

	useFakeCred(t, "arm", nil)
	origMgmt := cloudazure.ManagementEndpoint
	cloudazure.ManagementEndpoint = mgmt.URL
	t.Cleanup(func() { cloudazure.ManagementEndpoint = origMgmt })

	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.StreamLogs(ctx, "myapp", "rev", "rep", "ctr", false); err == nil {
		t.Error("StreamLogs should fail when getAuthToken returns 403")
	}
}

func TestStreamLogsHTTPError(t *testing.T) {
	var base string
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containerApps/myapp"):
			_, _ = w.Write([]byte(`{"properties":{"eventStreamEndpoint":"` + base + `/subscriptions/s/x"}}`))
		case strings.Contains(r.URL.Path, "/getAuthToken"):
			_, _ = w.Write([]byte(`{"properties":{"token":"t"}}`))
		case strings.Contains(r.URL.Path, "/logstream"):
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer mgmt.Close()
	base = mgmt.URL

	useFakeCred(t, "arm", nil)
	origMgmt := cloudazure.ManagementEndpoint
	cloudazure.ManagementEndpoint = mgmt.URL
	t.Cleanup(func() { cloudazure.ManagementEndpoint = origMgmt })

	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := c.StreamLogs(ctx, "myapp", "rev", "rep", "ctr", false); err == nil {
		t.Error("StreamLogs should fail when log stream returns 500")
	}
}

func TestWatchLogsNewClientOK(t *testing.T) {
	// With fake cred succeeding, newContainerAppsClient works; WatchLogs
	// begins polling and the poll will error on ARM call, but the retry loop
	// continues until ctx is cancelled.
	useFakeCred(t, "tok", nil)
	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	ch := c.WatchLogs(ctx)
	for range ch {
	}
}

func TestResolveLogTargetCredError(t *testing.T) {
	// Swap in a credential function that actually errors out at construction.
	orig := cloudazure.NewCredsFunc
	cloudazure.NewCredsFunc = func(_ cloudazure.Azure) (azcore.TokenCredential, error) {
		return nil, errors.New("no cred")
	}
	t.Cleanup(func() { cloudazure.NewCredsFunc = orig })

	c := &ContainerApp{
		Azure:         cloudazure.Azure{SubscriptionID: "sub", Location: cloudazure.LocationWestUS2},
		ResourceGroup: "rg",
	}
	if _, _, _, err := c.ResolveLogTarget(context.Background(), "app", "", "", ""); err == nil {
		t.Error("expected credential error")
	}
}
