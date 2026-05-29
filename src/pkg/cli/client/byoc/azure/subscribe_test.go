package azure

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	armappcontainersv3 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/acr"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// TestMain dials subscribePollInterval down for the entire test binary so
// the pollers tick fast. The write happens once on the main goroutine
// before any test runs, so it happens-before every poller's read — no
// per-test mutation, no race.
func TestMain(m *testing.M) {
	subscribePollInterval = time.Millisecond
	os.Exit(m.Run())
}

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

// drain collects SubscribeResponses with a short deadline. The producer
// no longer self-terminates (CD tracking and the allServicesTerminal exit
// were removed; cleanup happens via parent-ctx cancellation in the
// TailAndMonitor flow), so tests rely on the timeout to stop pollBuilds.
// 100ms at a 1ms poll interval is ~100 ticks per poller — far more than
// needed for state transitions to drain.
func drain(t *testing.T, ctx context.Context, in subscribeInputs) []*defangv1.SubscribeResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	var got []*defangv1.SubscribeResponse
	for resp, err := range subscribe(ctx, in) {
		if err != nil {
			t.Fatalf("drain err: %v", err)
		}
		got = append(got, resp)
	}
	return got
}

func TestSubscribe_AllRevisionsHealthy(t *testing.T) {
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
	got := drain(t, t.Context(), subscribeInputs{
		etag:      etag,
		services:  services,
		since:     time.Now(),
		revisions: rev,
		runs:      &fakeRunsClient{},
	})

	finals := finalStates(got, services)
	for _, svc := range services {
		if finals[svc] != defangv1.ServiceState_DEPLOYMENT_COMPLETED {
			t.Errorf("service %q final state = %v, want DEPLOYMENT_COMPLETED (got events: %+v)", svc, finals[svc], got)
		}
	}
}

func TestSubscribe_RevisionFailedTerminates(t *testing.T) {
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
	got := drain(t, t.Context(), subscribeInputs{
		etag:      etag,
		services:  []string{"web"},
		since:     time.Now(),
		revisions: rev,
		runs:      &fakeRunsClient{},
	})
	finals := finalStates(got, []string{"web"})
	if finals["web"] != defangv1.ServiceState_DEPLOYMENT_FAILED {
		t.Errorf("web final state = %v, want DEPLOYMENT_FAILED", finals["web"])
	}
}

func TestSubscribe_BuildStateEmitted(t *testing.T) {
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
	got := drain(t, t.Context(), subscribeInputs{
		etag:      etag,
		services:  []string{"web"},
		since:     time.Now(),
		revisions: rev,
		runs:      runs,
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
