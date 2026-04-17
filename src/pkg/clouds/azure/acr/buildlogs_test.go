package acr

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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

func armRunsClientFromCred(cred azcore.TokenCredential) (*armcontainerregistry.RunsClient, error) {
	return armcontainerregistry.NewRunsClient("sub", cred, nil)
}

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

func TestFetchLogContent(t *testing.T) {
	body := "line 1\nline 2\nline 3\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	got, err := fetchLogContent(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchLogContent error: %v", err)
	}
	if got != body {
		t.Errorf("content = %q, want %q", got, body)
	}
}

func TestFetchLogContentNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fetchLogContent(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404: %v", err)
	}
}

func TestFetchLogContentTruncation(t *testing.T) {
	// The helper caps reads at 1MB — verify it doesn't blow up on larger content.
	huge := strings.Repeat("x", 2*1024*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(huge))
	}))
	defer srv.Close()

	got, err := fetchLogContent(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchLogContent error: %v", err)
	}
	// Should be capped at the 1MB limit.
	if len(got) > 1024*1024 {
		t.Errorf("content length = %d, expected <= 1MB", len(got))
	}
}

func TestFetchLogContentCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchLogContent(ctx, srv.URL); err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestBuildLogEntry(t *testing.T) {
	e := BuildLogEntry{Service: "app", Line: "hello"}
	if e.Service != "app" || e.Line != "hello" {
		t.Errorf("BuildLogEntry fields = %+v", e)
	}
}

func TestWatchBuildLogsCredError(t *testing.T) {
	// With a bad credential, the goroutine should exit without emitting entries
	// and close the channel promptly.
	useFakeCred(t, "", errors.New("denied"))

	w := &BuildLogWatcher{
		Azure: azure.Azure{
			SubscriptionID: "sub",
			Location:       azure.LocationWestUS2,
		},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch := w.WatchBuildLogs(ctx)
	// With good credential but a non-existent RG, the watcher retries indefinitely
	// until ctx cancels. With bad credential (like here), clients may still be
	// constructed but will error on each call; the goroutine should still exit
	// when ctx expires without emitting anything.
	select {
	case entry, ok := <-ch:
		if ok && entry.Err != nil {
			// Acceptable: surfaced an error.
		}
		// Keep draining until close.
		for range ch {
		}
	case <-ctx.Done():
		// Give the goroutine a chance to see cancellation and close the channel.
	}
	// Drain to make sure the goroutine exits.
	for range ch {
	}
}

func TestStreamRunLogCancelled(t *testing.T) {
	// streamRunLog retries GetLogSasURL forever while it fails; with ctx
	// cancelled it should bail promptly.
	useFakeCred(t, "", errors.New("denied"))

	cred, err := azure.Azure{SubscriptionID: "sub"}.NewCreds()
	if err != nil {
		t.Fatalf("NewCreds: %v", err)
	}
	runsClient, err := armRunsClientFromCred(cred)
	if err != nil {
		t.Fatalf("runs client: %v", err)
	}

	w := &BuildLogWatcher{
		Azure: azure.Azure{
			SubscriptionID: "sub",
			Location:       azure.LocationWestUS2,
		},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out := make(chan BuildLogEntry)
	done := make(chan struct{})
	go func() {
		w.streamRunLog(ctx, runsClient, "rg", "registry", "run-id", "svc", out)
		close(done)
	}()
	go func() {
		for range out {
		}
	}()
	<-done
}

func TestWatchBuildLogsCancelled(t *testing.T) {
	// Verify the watcher exits promptly when ctx is cancelled, even when no
	// registry is ever found.
	useFakeCred(t, "tok", nil)

	w := &BuildLogWatcher{
		Azure: azure.Azure{
			SubscriptionID: "sub",
			Location:       azure.LocationWestUS2,
		},
		ResourceGroup: "rg",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	ch := w.WatchBuildLogs(ctx)
	for range ch {
	}
	// If we got here, the channel was closed — good.
}
