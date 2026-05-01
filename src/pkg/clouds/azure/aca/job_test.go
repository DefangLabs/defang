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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	armappcontainersv3 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	cloudazure "github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

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

func useFakeCred(t *testing.T, tok string, gerr error) {
	t.Helper()
	orig := cloudazure.NewCredsFunc
	cloudazure.NewCredsFunc = func(_ cloudazure.Azure) (azcore.TokenCredential, error) {
		return fakeCredential{token: tok, err: gerr}, nil
	}
	t.Cleanup(func() { cloudazure.NewCredsFunc = orig })
}

func useTestEndpoints(t *testing.T, mgmtURL, logAnalyticsURL string) {
	t.Helper()
	origMgmt := cloudazure.ManagementEndpoint
	origLA := logAnalyticsEndpoint
	cloudazure.ManagementEndpoint = mgmtURL
	if logAnalyticsURL != "" {
		logAnalyticsEndpoint = logAnalyticsURL
	}
	t.Cleanup(func() {
		cloudazure.ManagementEndpoint = origMgmt
		logAnalyticsEndpoint = origLA
	})
}

func TestJobStatusIsTerminal(t *testing.T) {
	tests := []struct {
		state armappcontainersv3.JobExecutionRunningState
		want  bool
	}{
		{armappcontainersv3.JobExecutionRunningStateSucceeded, true},
		{armappcontainersv3.JobExecutionRunningStateFailed, true},
		{armappcontainersv3.JobExecutionRunningStateStopped, true},
		{armappcontainersv3.JobExecutionRunningStateDegraded, true},
		{armappcontainersv3.JobExecutionRunningStateRunning, false},
		{armappcontainersv3.JobExecutionRunningStateProcessing, false},
	}
	for _, tt := range tests {
		s := &JobStatus{Status: tt.state}
		if got := s.IsTerminal(); got != tt.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestJobStatusIsSuccess(t *testing.T) {
	s := &JobStatus{Status: armappcontainersv3.JobExecutionRunningStateSucceeded}
	if !s.IsSuccess() {
		t.Error("Succeeded state should be success")
	}
	s.Status = armappcontainersv3.JobExecutionRunningStateFailed
	if s.IsSuccess() {
		t.Error("Failed state should not be success")
	}
}

func TestForwardStream(t *testing.T) {
	ctx := context.Background()
	ch := make(chan LogEntry, 3)
	ch <- LogEntry{Message: "a"}
	ch <- LogEntry{Message: "b"}
	ch <- LogEntry{Message: "c"}
	close(ch)

	var got []string
	gotLines, keepGoing := forwardStream(ctx, ch, func(msg string, err error) bool {
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		got = append(got, msg)
		return true
	})
	if !gotLines {
		t.Error("gotLines should be true")
	}
	if !keepGoing {
		t.Error("keepGoing should be true after drain")
	}
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("forwarded = %v", got)
	}
}

func TestForwardStreamEmpty(t *testing.T) {
	ctx := context.Background()
	ch := make(chan LogEntry)
	close(ch)
	gotLines, keepGoing := forwardStream(ctx, ch, func(string, error) bool { return true })
	if gotLines {
		t.Error("gotLines should be false for empty stream")
	}
	if !keepGoing {
		t.Error("keepGoing should be true for empty stream")
	}
}

func TestForwardStreamErrorEntry(t *testing.T) {
	ctx := context.Background()
	ch := make(chan LogEntry, 2)
	ch <- LogEntry{Err: context.Canceled}
	ch <- LogEntry{Message: "after err"}
	close(ch)

	var sawErr bool
	var msgCount int
	gotLines, keepGoing := forwardStream(ctx, ch, func(msg string, err error) bool {
		if err != nil {
			sawErr = true
		} else {
			msgCount++
		}
		return true
	})
	if !sawErr {
		t.Error("expected error to be forwarded")
	}
	if msgCount != 1 {
		t.Errorf("msgCount = %d, want 1", msgCount)
	}
	if !gotLines {
		t.Error("gotLines should be true (non-error line was forwarded)")
	}
	if !keepGoing {
		t.Error("keepGoing should be true")
	}
}

func TestForwardStreamEarlyExit(t *testing.T) {
	ctx := context.Background()
	ch := make(chan LogEntry, 3)
	ch <- LogEntry{Message: "a"}
	ch <- LogEntry{Message: "b"}
	ch <- LogEntry{Message: "c"}
	close(ch)

	count := 0
	gotLines, keepGoing := forwardStream(ctx, ch, func(string, error) bool {
		count++
		return count < 2 // stop after second call
	})
	if keepGoing {
		t.Error("keepGoing should be false when yield returns false")
	}
	if !gotLines {
		t.Error("gotLines should be true")
	}
}

func TestForwardStreamCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := make(chan LogEntry, 1)
	ch <- LogEntry{Message: "ignored"}
	close(ch)

	_, keepGoing := forwardStream(ctx, ch, func(string, error) bool { return true })
	if keepGoing {
		t.Error("keepGoing should be false when context is cancelled")
	}
}

func newTestJob() *Job {
	return &Job{
		Azure: cloudazure.Azure{
			SubscriptionID: "sub",
			Location:       cloudazure.LocationWestUS2,
		},
		ResourceGroup: "rg",
	}
}

func TestGetJobAuthToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "Microsoft.App/jobs/defang-cd/getAuthToken") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"properties":{"token":"jwt-here"}}`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	j := newTestJob()
	tok, err := j.getJobAuthToken(context.Background())
	if err != nil {
		t.Fatalf("getJobAuthToken: %v", err)
	}
	if tok != "jwt-here" {
		t.Errorf("token = %q", tok)
	}
}

func TestGetCDContainerLogStreamURLRunning(t *testing.T) {
	// replicas response: one container in Running state.
	resp := `{"value":[{"properties":{"containers":[
		{"name":"defang-cd","runningState":"Running","logStreamEndpoint":"https://example/stream"}
	]}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "executions/exec-1/replicas") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	j := newTestJob()
	url, err := j.getCDContainerLogStreamURL(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("getCDContainerLogStreamURL: %v", err)
	}
	if url != "https://example/stream" {
		t.Errorf("url = %q", url)
	}
}

func TestGetCDContainerLogStreamURLWaiting(t *testing.T) {
	// Container is still Waiting — expect empty URL (caller retries).
	resp := `{"value":[{"properties":{"containers":[
		{"name":"defang-cd","runningState":"Waiting","logStreamEndpoint":"https://example/stream"}
	]}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	j := newTestJob()
	url, err := j.getCDContainerLogStreamURL(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if url != "" {
		t.Errorf("URL should be empty while container is Waiting, got %q", url)
	}
}

func TestGetCDContainerLogStreamURLMissingContainer(t *testing.T) {
	resp := `{"value":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	j := newTestJob()
	url, err := j.getCDContainerLogStreamURL(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if url != "" {
		t.Errorf("URL should be empty when replicas list is empty")
	}
}

func TestGetCDContainerLogStreamURLHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	j := newTestJob()
	if _, err := j.getCDContainerLogStreamURL(context.Background(), "exec-1"); err == nil {
		t.Error("expected error for 401")
	}
}

func TestStreamJobExecutionLogsNoReplica(t *testing.T) {
	// Empty replicas list — streamJobExecutionLogs should surface an error so
	// the caller can retry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer srv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, srv.URL, "")

	j := newTestJob()
	if _, err := j.streamJobExecutionLogs(context.Background(), "exec-1", 0); err == nil {
		t.Error("expected error when no replica")
	}
}

func TestStreamJobExecutionLogs(t *testing.T) {
	streamBody := "first\nsecond\nthird\n"

	// Chain two servers: the first serves replicas + auth token, the second
	// serves the logstream. The replicas response points at the stream server.
	var streamSrv *httptest.Server
	streamSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("follow") != "true" {
			t.Errorf("follow = %q, want true", r.URL.Query().Get("follow"))
		}
		_, _ = w.Write([]byte(streamBody))
	}))
	defer streamSrv.Close()

	mgmtSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "getAuthToken"):
			_, _ = w.Write([]byte(`{"properties":{"token":"stream-tok"}}`))
		case strings.Contains(r.URL.Path, "replicas"):
			resp := `{"value":[{"properties":{"containers":[
				{"name":"defang-cd","runningState":"Running","logStreamEndpoint":"` + streamSrv.URL + `"}
			]}}]}`
			_, _ = w.Write([]byte(resp))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mgmtSrv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, mgmtSrv.URL, "")

	j := newTestJob()
	ch, err := j.streamJobExecutionLogs(context.Background(), "exec-1", 0)
	if err != nil {
		t.Fatalf("streamJobExecutionLogs: %v", err)
	}
	var got []string
	for entry := range ch {
		if entry.Err != nil {
			t.Errorf("entry err: %v", entry.Err)
			continue
		}
		got = append(got, entry.Message)
	}
	if len(got) != 3 || got[0] != "first" || got[2] != "third" {
		t.Errorf("got lines %v", got)
	}
}

func TestStreamJobExecutionLogsBackfill(t *testing.T) {
	// Verify that backfillLines > 0 adds tailLines query param.
	var gotTail string
	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTail = r.URL.Query().Get("tailLines")
		_, _ = w.Write([]byte("x\n"))
	}))
	defer streamSrv.Close()

	mgmtSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "getAuthToken"):
			_, _ = w.Write([]byte(`{"properties":{"token":"tok"}}`))
		case strings.Contains(r.URL.Path, "replicas"):
			resp := `{"value":[{"properties":{"containers":[
				{"name":"defang-cd","runningState":"Running","logStreamEndpoint":"` + streamSrv.URL + `"}
			]}}]}`
			_, _ = w.Write([]byte(resp))
		}
	}))
	defer mgmtSrv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, mgmtSrv.URL, "")

	j := newTestJob()
	ch, err := j.streamJobExecutionLogs(context.Background(), "exec-1", 250)
	if err != nil {
		t.Fatalf("streamJobExecutionLogs: %v", err)
	}
	for range ch {
	}
	if gotTail != "250" {
		t.Errorf("tailLines = %q, want 250", gotTail)
	}
}

func TestStreamJobExecutionLogsHTTPFailure(t *testing.T) {
	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer streamSrv.Close()

	mgmtSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "getAuthToken"):
			_, _ = w.Write([]byte(`{"properties":{"token":"t"}}`))
		case strings.Contains(r.URL.Path, "replicas"):
			resp := `{"value":[{"properties":{"containers":[
				{"name":"defang-cd","runningState":"Running","logStreamEndpoint":"` + streamSrv.URL + `"}
			]}}]}`
			_, _ = w.Write([]byte(resp))
		}
	}))
	defer mgmtSrv.Close()

	useFakeCred(t, "arm", nil)
	useTestEndpoints(t, mgmtSrv.URL, "")

	j := newTestJob()
	if _, err := j.streamJobExecutionLogs(context.Background(), "exec-1", 0); err == nil {
		t.Error("expected error for 403 from stream endpoint")
	}
}

func TestStreamJobExecutionLogsCredError(t *testing.T) {
	mgmtSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"value":[{"properties":{"containers":[
			{"name":"defang-cd","runningState":"Running","logStreamEndpoint":"https://example/x"}
		]}}]}`
		_, _ = w.Write([]byte(resp))
	}))
	defer mgmtSrv.Close()

	useFakeCred(t, "", errors.New("cred fail"))
	useTestEndpoints(t, mgmtSrv.URL, "")

	j := newTestJob()
	// First call (replicas list) goes through ArmToken → fails.
	if _, err := j.streamJobExecutionLogs(context.Background(), "exec-1", 0); err == nil {
		t.Error("expected credential error")
	}
}

func TestReadJobLogs(t *testing.T) {
	// Log Analytics query returns two rows of (timestamp, line).
	laBody := `{"tables":[{"rows":[
		["2026-04-17T16:00:00Z","hello"],
		["2026-04-17T16:00:01Z","world"]
	]}]}`
	laSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/workspaces/") || !strings.HasSuffix(r.URL.Path, "/query") {
			t.Errorf("LA path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("LA method = %s", r.Method)
		}
		_, _ = w.Write([]byte(laBody))
	}))
	defer laSrv.Close()

	// The workspace customerID is fetched from ARM via the SDK's workspaces client,
	// which is hard to mock. We bypass by only testing the fetchLogsFromWorkspace
	// helper directly with a pre-known workspaceID (via a small thin wrapper).
	// The helper is private but we can still drive the Log Analytics endpoint
	// through the public ReadJobLogs path with a pre-populated workspace.
	// Since we cannot populate the SDK client response, we at least verify the
	// LA path by calling fetchLogsFromWorkspace indirectly.
	useFakeCred(t, "la-tok", nil)
	useTestEndpoints(t, "http://unused", laSrv.URL)

	j := newTestJob()
	// Call the low-level fetch function directly to avoid the SDK workspace lookup.
	got, err := j.fetchLogsByWorkspaceID(context.Background(), "workspace-guid", "exec-1")
	if err != nil {
		t.Fatalf("fetchLogsByWorkspaceID: %v", err)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("logs = %q", got)
	}
}

func TestReadJobLogsTokenError(t *testing.T) {
	useFakeCred(t, "", errors.New("token denied"))
	j := newTestJob()
	if _, err := j.fetchLogsByWorkspaceID(context.Background(), "ws", "exec"); err == nil {
		t.Error("expected token error")
	}
}

func TestReadJobLogsHTTPError(t *testing.T) {
	laSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer laSrv.Close()

	useFakeCred(t, "tok", nil)
	useTestEndpoints(t, "http://unused", laSrv.URL)

	j := newTestJob()
	if _, err := j.fetchLogsByWorkspaceID(context.Background(), "ws", "exec"); err == nil {
		t.Error("expected error for 500")
	}
}

func TestReadJobLogsBadJSON(t *testing.T) {
	laSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer laSrv.Close()

	useFakeCred(t, "tok", nil)
	useTestEndpoints(t, "http://unused", laSrv.URL)

	j := newTestJob()
	if _, err := j.fetchLogsByWorkspaceID(context.Background(), "ws", "exec"); err == nil {
		t.Error("expected decode error")
	}
}

func TestNewClients(t *testing.T) {
	useFakeCred(t, "tok", nil)
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if c, err := j.newManagedEnvironmentsClient(); err != nil || c == nil {
		t.Errorf("newManagedEnvironmentsClient: %v, client=%v", err, c)
	}
	if c, err := j.newJobsClient(); err != nil || c == nil {
		t.Errorf("newJobsClient: %v, client=%v", err, c)
	}
	if c, err := j.newJobsExecutionsClient(); err != nil || c == nil {
		t.Errorf("newJobsExecutionsClient: %v, client=%v", err, c)
	}
}

func TestSetUpManagedIdentityPreconditions(t *testing.T) {
	useFakeCred(t, "tok", nil)
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	// SystemPrincipalID is not set.
	if err := j.SetUpManagedIdentity(context.Background(), "acct"); err == nil {
		t.Error("expected error when SystemPrincipalID is empty")
	}

	// idempotent when identitySetUp is true.
	j.identitySetUp = true
	if err := j.SetUpManagedIdentity(context.Background(), "acct"); err != nil {
		t.Errorf("identity already set up should short-circuit, got %v", err)
	}
}

func TestSetUpEnvironmentShortCircuit(t *testing.T) {
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg", EnvironmentID: "/already"}
	if err := j.SetUpEnvironment(context.Background()); err != nil {
		t.Errorf("SetUpEnvironment should short-circuit when EnvironmentID is set, got %v", err)
	}
}

func TestSetUpJobMissingEnvironment(t *testing.T) {
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if err := j.SetUpJob(context.Background(), "image", nil); err == nil {
		t.Error("SetUpJob should fail when EnvironmentID is empty")
	}
}

func TestStartJobExecutionCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if _, err := j.StartJobExecution(context.Background(), JobRequest{
		Image:   "img",
		Command: []string{"/bin/true"},
	}); err == nil {
		t.Error("expected cred error")
	}
}

func TestTailJobLogsCancelled(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	seq, err := j.TailJobLogs(ctx, "exec-1")
	if err != nil {
		t.Fatalf("TailJobLogs: %v", err)
	}
	for range seq {
		// drain
	}
}

func TestGetJobExecutionStatusCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if _, err := j.GetJobExecutionStatus(context.Background(), "exec"); err == nil {
		t.Error("expected cred error")
	}
}

func TestReadJobLogsCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if _, err := j.ReadJobLogs(context.Background(), "exec"); err == nil {
		t.Error("expected cred error")
	}
}

func TestGetLogAnalyticsTokenCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if _, err := j.getLogAnalyticsToken(context.Background()); err == nil {
		t.Error("expected cred error")
	}
}

func TestGetLogWorkspaceCustomerIDCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if _, err := j.getLogWorkspaceCustomerID(context.Background()); err == nil {
		t.Error("expected cred error")
	}
}

func TestSetUpLogWorkspaceCredError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	if _, _, err := j.setUpLogWorkspace(context.Background()); err == nil {
		t.Error("expected cred error")
	}
}

func TestFetchLogsFromWorkspaceSDKError(t *testing.T) {
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	// fetchLogsFromWorkspace first calls getLogWorkspaceCustomerID which uses
	// the SDK (will fail), then bails.
	if _, err := j.fetchLogsFromWorkspace(context.Background(), "exec"); err == nil {
		t.Error("expected error from fetchLogsFromWorkspace")
	}
}

func TestFetchLogsFromWorkspaceViaTailJobLogs(t *testing.T) {
	// Exercise the non-error return path in forwardStream by producing one
	// terminal status and no logs.
	useFakeCred(t, "", errors.New("denied"))
	j := &Job{Azure: cloudazure.Azure{SubscriptionID: "sub"}, ResourceGroup: "rg"}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	seq, err := j.TailJobLogs(ctx, "exec")
	if err != nil {
		t.Fatalf("TailJobLogs: %v", err)
	}
	for range seq {
	}
}
