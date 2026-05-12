package azure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	armappcontainersv3 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/acr"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type fakeRevisionsClient struct {
	mu     sync.Mutex
	states map[string][]*aca.RevisionState // revisionName → script of states to return on successive calls
	calls  map[string]int
}

func (f *fakeRevisionsClient) GetRevisionState(_ context.Context, _, revisionName string) (*aca.RevisionState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	idx := f.calls[revisionName]
	f.calls[revisionName] = idx + 1
	seq, ok := f.states[revisionName]
	if !ok || len(seq) == 0 {
		return &aca.RevisionState{NotFound: true}, nil
	}
	if idx >= len(seq) {
		idx = len(seq) - 1
	}
	return seq[idx], nil
}

type fakeRunsClient struct {
	mu      sync.Mutex
	updates [][]acr.RunInfo
	calls   int
}

func (f *fakeRunsClient) ListRunsSince(_ context.Context, _ time.Time) ([]acr.RunInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	f.calls++
	if idx >= len(f.updates) {
		if len(f.updates) == 0 {
			return nil, nil
		}
		return f.updates[len(f.updates)-1], nil
	}
	return f.updates[idx], nil
}

type fakeJobClient struct {
	mu       sync.Mutex
	statuses []*aca.JobStatus // returned on successive calls
	calls    int
}

func (f *fakeJobClient) GetJobExecutionStatus(_ context.Context, _ string) (*aca.JobStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.calls
	f.calls++
	if idx >= len(f.statuses) {
		idx = len(f.statuses) - 1
	}
	if idx < 0 {
		return nil, errors.New("no status")
	}
	return f.statuses[idx], nil
}

// withFastPoll forces subscribePollInterval down to a millisecond for tests
// and restores it on cleanup.
func withFastPoll(t *testing.T) {
	t.Helper()
	orig := subscribePollInterval
	subscribePollInterval = time.Millisecond
	t.Cleanup(func() { subscribePollInterval = orig })
}

// drain collects all SubscribeResponses (and any error) from the iterator.
// Stops at the first error or when the iterator is exhausted.
func drain(t *testing.T, ctx context.Context, in subscribeInputs) ([]*defangv1.SubscribeResponse, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var got []*defangv1.SubscribeResponse
	var firstErr error
	for resp, err := range subscribe(ctx, in) {
		if err != nil {
			firstErr = err
			break
		}
		got = append(got, resp)
	}
	return got, firstErr
}

func TestSubscribe_AllHealthyAfterCdSuccess(t *testing.T) {
	withFastPoll(t)

	etag := "abc123"
	services := []string{"web", "worker"}
	rev := &fakeRevisionsClient{
		states: map[string][]*aca.RevisionState{
			"web--" + etag: {
				{NotFound: true},
				healthyRevision(),
			},
			"worker--" + etag: {
				{NotFound: true},
				healthyRevision(),
			},
		},
	}
	job := &fakeJobClient{
		statuses: []*aca.JobStatus{
			{Status: armappcontainersv3.JobExecutionRunningStateRunning},
			{Status: armappcontainersv3.JobExecutionRunningStateSucceeded},
		},
	}
	got, err := drain(t, t.Context(), subscribeInputs{
		etag:      etag,
		services:  services,
		since:     time.Now(),
		revisions: rev,
		runs:      &fakeRunsClient{},
		job:       job,
		cdRunID:   "cd-run",
	})
	if err != nil {
		t.Fatalf("drain err: %v", err)
	}

	finals := finalStates(got, services)
	for _, svc := range services {
		if finals[svc] != defangv1.ServiceState_DEPLOYMENT_COMPLETED {
			t.Errorf("service %q final state = %v, want DEPLOYMENT_COMPLETED (got events: %+v)", svc, finals[svc], got)
		}
	}
}

func TestSubscribe_CdFailurePropagates(t *testing.T) {
	withFastPoll(t)

	rev := &fakeRevisionsClient{}
	job := &fakeJobClient{
		statuses: []*aca.JobStatus{
			{Status: armappcontainersv3.JobExecutionRunningStateFailed, ErrorMessage: "pulumi crashed"},
		},
	}
	_, err := drain(t, t.Context(), subscribeInputs{
		etag:      "e",
		services:  []string{"web"},
		since:     time.Now(),
		revisions: rev,
		runs:      &fakeRunsClient{},
		job:       job,
		cdRunID:   "cd-run",
	})
	if err == nil {
		t.Fatal("expected error from CD failure, got nil")
	}
	var dfe client.ErrDeploymentFailed
	if !errors.As(err, &dfe) {
		t.Fatalf("err = %v, want ErrDeploymentFailed", err)
	}
}

func TestSubscribe_RevisionFailedTerminates(t *testing.T) {
	withFastPoll(t)

	etag := "fail123"
	rev := &fakeRevisionsClient{
		states: map[string][]*aca.RevisionState{
			"web--" + etag: {
				{
					ProvisioningState: armappcontainersv3.RevisionProvisioningStateFailed,
					ProvisioningError: "image pull error",
				},
			},
		},
	}
	job := &fakeJobClient{
		statuses: []*aca.JobStatus{{Status: armappcontainersv3.JobExecutionRunningStateRunning}},
	}
	got, err := drain(t, t.Context(), subscribeInputs{
		etag:      etag,
		services:  []string{"web"},
		since:     time.Now(),
		revisions: rev,
		runs:      &fakeRunsClient{},
		job:       job,
		cdRunID:   "cd-run",
	})
	if err != nil {
		t.Fatalf("drain err: %v", err)
	}
	finals := finalStates(got, []string{"web"})
	if finals["web"] != defangv1.ServiceState_DEPLOYMENT_FAILED {
		t.Errorf("web final state = %v, want DEPLOYMENT_FAILED", finals["web"])
	}
}

func TestSubscribe_BuildStateEmitted(t *testing.T) {
	withFastPoll(t)

	etag := "b1"
	rev := &fakeRevisionsClient{
		states: map[string][]*aca.RevisionState{
			"web--" + etag: {healthyRevision()},
		},
	}
	runs := &fakeRunsClient{
		updates: [][]acr.RunInfo{
			{{RunID: "r1", Task: "web", Status: armcontainerregistry.RunStatusRunning}},
			{{RunID: "r1", Task: "web", Status: armcontainerregistry.RunStatusSucceeded}},
		},
	}
	job := &fakeJobClient{
		statuses: []*aca.JobStatus{
			{Status: armappcontainersv3.JobExecutionRunningStateRunning},
			{Status: armappcontainersv3.JobExecutionRunningStateSucceeded},
		},
	}
	got, _ := drain(t, t.Context(), subscribeInputs{
		etag:      etag,
		services:  []string{"web"},
		since:     time.Now(),
		revisions: rev,
		runs:      runs,
		job:       job,
		cdRunID:   "cd-run",
	})
	var sawBuild bool
	for _, r := range got {
		if r.Name == "web" && (r.State == defangv1.ServiceState_BUILD_RUNNING || r.State == defangv1.ServiceState_BUILD_STOPPING) {
			sawBuild = true
			break
		}
	}
	if !sawBuild {
		t.Errorf("expected at least one BUILD_* event for web, got: %+v", got)
	}
}

func TestMapRevisionState_NotFound(t *testing.T) {
	state, _, terminal := mapRevisionState(nil)
	if state != defangv1.ServiceState_UPDATE_QUEUED || terminal {
		t.Errorf("nil → (%v, terminal=%v), want UPDATE_QUEUED, terminal=false", state, terminal)
	}
	state, _, terminal = mapRevisionState(&aca.RevisionState{NotFound: true})
	if state != defangv1.ServiceState_UPDATE_QUEUED || terminal {
		t.Errorf("NotFound → (%v, terminal=%v), want UPDATE_QUEUED, terminal=false", state, terminal)
	}
}

func TestMapRevisionState_Healthy(t *testing.T) {
	state, _, terminal := mapRevisionState(healthyRevision())
	if state != defangv1.ServiceState_DEPLOYMENT_COMPLETED || !terminal {
		t.Errorf("healthy → (%v, terminal=%v), want DEPLOYMENT_COMPLETED, terminal=true", state, terminal)
	}
}

func TestMapRevisionState_FailedProvisioning(t *testing.T) {
	state, _, terminal := mapRevisionState(&aca.RevisionState{
		ProvisioningState: armappcontainersv3.RevisionProvisioningStateFailed,
	})
	if state != defangv1.ServiceState_DEPLOYMENT_FAILED || !terminal {
		t.Errorf("failed → (%v, terminal=%v), want DEPLOYMENT_FAILED, terminal=true", state, terminal)
	}
}

func TestMapRunStatus(t *testing.T) {
	tests := []struct {
		in   armcontainerregistry.RunStatus
		want defangv1.ServiceState
	}{
		{armcontainerregistry.RunStatusQueued, defangv1.ServiceState_BUILD_QUEUED},
		{armcontainerregistry.RunStatusRunning, defangv1.ServiceState_BUILD_RUNNING},
		{armcontainerregistry.RunStatusSucceeded, defangv1.ServiceState_BUILD_STOPPING},
		{armcontainerregistry.RunStatusFailed, defangv1.ServiceState_BUILD_FAILED},
		{armcontainerregistry.RunStatusCanceled, defangv1.ServiceState_BUILD_FAILED},
		{armcontainerregistry.RunStatusError, defangv1.ServiceState_BUILD_FAILED},
		{armcontainerregistry.RunStatusTimeout, defangv1.ServiceState_BUILD_FAILED},
	}
	for _, tt := range tests {
		got, _ := mapRunStatus(tt.in)
		if got != tt.want {
			t.Errorf("mapRunStatus(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func healthyRevision() *aca.RevisionState {
	return &aca.RevisionState{
		ProvisioningState: armappcontainersv3.RevisionProvisioningStateProvisioned,
		RunningState:      armappcontainersv3.RevisionRunningStateRunning,
		HealthState:       armappcontainersv3.RevisionHealthStateHealthy,
	}
}

// finalStates returns the last observed state per service from a stream of responses.
func finalStates(resps []*defangv1.SubscribeResponse, services []string) map[string]defangv1.ServiceState {
	out := map[string]defangv1.ServiceState{}
	for _, s := range services {
		out[s] = defangv1.ServiceState_NOT_SPECIFIED
	}
	for _, r := range resps {
		if _, ok := out[r.Name]; ok {
			out[r.Name] = r.State
		}
	}
	return out
}
