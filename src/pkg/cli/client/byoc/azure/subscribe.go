package azure

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"sync"
	"time"

	armappcontainersv3 "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/state"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/aca"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/acr"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure/loadbalancer"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// Polling cadence. Container Apps revisions normally reach Healthy in
// 30–90s; 5s is the same cadence BuildLogWatcher uses for ACR runs. Exposed
// as a var (not const) so tests can dial it down to a millisecond.
var subscribePollInterval = 5 * time.Second

// subscribeRevisionsClient and subscribeRunsClient narrow the surface the
// orchestrator uses to one method each so tests can stub them without
// pulling in the full ACA / ACR SDK clients.
type subscribeRevisionsClient interface {
	GetRevisionState(ctx context.Context, appName, revisionName string) (*aca.RevisionState, error)
}

type subscribeRunsClient interface {
	ListRunsSince(ctx context.Context, since time.Time) ([]acr.RunInfo, error)
}

type subscribeStateClient interface {
	GetPulumiState(ctx context.Context) (*state.PulumiState, error)
}

type subscribeLoadBalancerClient interface {
	AllBackendsHealthy(ctx context.Context, resourceID string, since time.Time) (bool, error)
}

// Subscribe implements client.Provider. It polls ACR Task runs for builds and
// uses the Pulumi component graph to select the deployment signal actually
// materialized for each service: a Container Apps revision or VMSS load
// balancer health.
//
// CD-job state is intentionally NOT polled here. The CLI's TailAndMonitor
// (pkg/cli/tailAndMonitor.go) runs WaitForCdTaskExit in parallel, which
// polls ByocAzure.GetDeploymentStatus and surfaces CD success/failure
// independently. Tracking it in both places would mean reporting the same
// failure twice and gating revision-completed events on a CD success that
// the caller already observes via a separate goroutine.
//
// As a consequence this producer does not self-terminate: pollBuilds has
// no intrinsic exit condition (no transitions for a tick ≠ no more builds
// coming), so the aggregator only exits when the consumer cancels the
// parent ctx or stops the iterator. In the TailAndMonitor flow the parent
// ctx is cancelled when the cobra command's ctx is — the transient cost
// between WaitServiceState returning and command exit is a few extra ACR
// list calls.
func (b *ByocAzure) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (iter.Seq2[*defangv1.SubscribeResponse, error], error) {
	if b.cdRunID == "" {
		return nil, errors.New("Subscribe: no active deployment (CdCommand or Deploy must be called first)")
	}
	etag := b.cdEtag
	if req.Etag != "" && req.Etag != etag {
		return nil, fmt.Errorf("Subscribe: requested etag %q does not match active deployment etag %q", req.Etag, etag)
	}
	if err := b.setUpLocation(); err != nil {
		return nil, err
	}
	if len(req.Services) == 0 {
		return nil, errors.New("Subscribe: no services to monitor")
	}

	projectRG := b.projectResourceGroupName(req.Project)
	revisions := &aca.ContainerApp{Azure: b.driver.Azure, ResourceGroup: projectRG}
	runs := &acr.RunsLister{Azure: b.driver.Azure, ResourceGroup: projectRG}
	loadBalancers := &loadbalancer.HealthClient{Azure: b.driver.Azure}
	stateReader := &azureStateReader{
		download: b.driver.DownloadBlob,
		blobName: path.Join(".pulumi/stacks", req.Project, b.PulumiStack+".json"),
	}

	since := b.cdStart
	if since.IsZero() {
		since = time.Now().Add(-2 * time.Minute)
	}

	return subscribe(ctx, subscribeInputs{
		etag:          etag,
		services:      req.Services,
		since:         since,
		revisions:     revisions,
		runs:          runs,
		state:         stateReader,
		loadBalancers: loadBalancers,
	}), nil
}

type subscribeInputs struct {
	etag          string
	services      []string
	since         time.Time
	revisions     subscribeRevisionsClient
	runs          subscribeRunsClient
	state         subscribeStateClient
	loadBalancers subscribeLoadBalancerClient
}

func subscribe(ctx context.Context, in subscribeInputs) iter.Seq2[*defangv1.SubscribeResponse, error] {
	return func(yield func(*defangv1.SubscribeResponse, error) bool) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		eventCh := make(chan *defangv1.SubscribeResponse)
		var wg sync.WaitGroup

		wg.Add(1)
		go pollBuilds(ctx, in.runs, in.services, in.since, eventCh, &wg)

		for _, service := range in.services {
			wg.Add(1)
			go pollService(ctx, in, service, eventCh, &wg)
		}

		go func() {
			wg.Wait()
			close(eventCh)
		}()

		for resp := range eventCh {
			if !yield(resp, nil) {
				return
			}
		}
	}
}

type blobObject struct {
	name string
	size int64
}

func (o blobObject) Name() string { return o.name }
func (o blobObject) Size() int64  { return o.size }

type azureStateReader struct {
	download func(context.Context, string) ([]byte, error)
	blobName string
}

func (r *azureStateReader) GetPulumiState(ctx context.Context) (*state.PulumiState, error) {
	data, err := r.download(ctx, r.blobName)
	if err != nil {
		return nil, err
	}
	return state.ParsePulumiStateFile(ctx, blobObject{name: r.blobName, size: int64(len(data))},
		func(context.Context, string) ([]byte, error) { return data, nil })
}

const (
	azureServiceType      = "defang-azure:index:Service"
	azureContainerAppType = "azure-native:app:ContainerApp"
	azureVMScaleSetType   = "azure-native:compute:VirtualMachineScaleSet"
	azureLoadBalancerType = "azure-native:network:LoadBalancer"
)

func pollService(ctx context.Context, in subscribeInputs, service string, out chan<- *defangv1.SubscribeResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	// Kept for narrow unit callers; production always supplies the state reader.
	if in.state == nil {
		wg.Add(1)
		pollRevision(ctx, in.revisions, service, in.etag, out, wg)
		return
	}

	ticker := time.NewTicker(subscribePollInterval)
	defer ticker.Stop()
	for {
		pulumiState, err := in.state.GetPulumiState(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			term.Debugf("Subscribe: reading Pulumi state for %q: %v", service, err)
		} else if pulumiState != nil {
			var component *state.Resource
			for i := range pulumiState.Resources {
				resource := &pulumiState.Resources[i]
				if resource.Type == azureServiceType && resource.Name == service {
					component = resource
					break
				}
			}
			if component != nil {
				var hasContainerApp, hasVMSS bool
				var vmssModified time.Time
				var loadBalancerID string
				for _, resource := range pulumiState.Descendants(component.URN) {
					switch resource.Type {
					case azureContainerAppType:
						hasContainerApp = true
					case azureVMScaleSetType:
						hasVMSS = true
						vmssModified = resource.Modified
					case azureLoadBalancerType:
						loadBalancerID = resource.ID
					}
				}
				switch {
				case hasVMSS && loadBalancerID != "" &&
					(vmssModified.IsZero() || !vmssModified.Before(in.since.Add(-5*time.Second))):
					pollLoadBalancer(ctx, in.loadBalancers, service, loadBalancerID, vmssModified, out)
					return
				case hasContainerApp:
					wg.Add(1)
					pollRevision(ctx, in.revisions, service, in.etag, out, wg)
					return
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func pollLoadBalancer(ctx context.Context, c subscribeLoadBalancerClient, service, resourceID string, since time.Time, out chan<- *defangv1.SubscribeResponse) {
	ticker := time.NewTicker(subscribePollInterval)
	defer ticker.Stop()
	status := "waiting for load balancer health"
	select {
	case out <- &defangv1.SubscribeResponse{Name: service, Status: status, State: defangv1.ServiceState_DEPLOYMENT_PENDING}:
	case <-ctx.Done():
		return
	}
	for {
		healthy, err := c.AllBackendsHealthy(ctx, resourceID, since)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			term.Debugf("Subscribe: load balancer %q health error: %v", resourceID, err)
		} else if healthy {
			select {
			case out <- &defangv1.SubscribeResponse{Name: service, Status: "healthy", State: defangv1.ServiceState_DEPLOYMENT_COMPLETED}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func pollRevision(ctx context.Context, c subscribeRevisionsClient, service, etag string, out chan<- *defangv1.SubscribeResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	revisionName := service + "--" + etag

	ticker := time.NewTicker(subscribePollInterval)
	defer ticker.Stop()

	var lastState defangv1.ServiceState
	var lastStatus string

	emit := func(state defangv1.ServiceState, status string) bool {
		if state == lastState && status == lastStatus {
			return true
		}
		lastState = state
		lastStatus = status
		select {
		case out <- &defangv1.SubscribeResponse{
			Name:   service,
			Status: status,
			State:  state,
		}:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for {
		state, err := c.GetRevisionState(ctx, service, revisionName)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			term.Debugf("Subscribe: revision %q error: %v", revisionName, err)
		} else {
			respState, status, terminal := mapRevisionState(state)
			if !emit(respState, status) {
				return
			}
			if terminal {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// mapRevisionState reduces (provisioningState, runningState, healthState)
// to a single ServiceState and a short human-readable status. The third
// return value is true when this is a final state — callers stop polling.
func mapRevisionState(s *aca.RevisionState) (defangv1.ServiceState, string, bool) {
	if s == nil || s.NotFound {
		// Revision not yet created by Pulumi.
		return defangv1.ServiceState_UPDATE_QUEUED, "waiting for revision", false
	}
	switch s.ProvisioningState {
	case armappcontainersv3.RevisionProvisioningStateFailed:
		msg := "revision provisioning failed"
		if s.ProvisioningError != "" {
			msg = s.ProvisioningError
		}
		return defangv1.ServiceState_DEPLOYMENT_FAILED, msg, true
	case armappcontainersv3.RevisionProvisioningStateProvisioning, "":
		// "" covers the brief window where the revision exists but
		// hasn't been classified yet.
		return defangv1.ServiceState_DEPLOYMENT_PENDING, "provisioning", false
	case armappcontainersv3.RevisionProvisioningStateDeprovisioning,
		armappcontainersv3.RevisionProvisioningStateDeprovisioned:
		return defangv1.ServiceState_DEPLOYMENT_FAILED, "revision was deprovisioned", true
	}

	// ProvisioningState == Provisioned at this point — gate on running/health.
	switch s.RunningState {
	case armappcontainersv3.RevisionRunningStateFailed,
		armappcontainersv3.RevisionRunningStateDegraded:
		return defangv1.ServiceState_DEPLOYMENT_FAILED, "revision " + string(s.RunningState), true
	case armappcontainersv3.RevisionRunningStateStopped:
		return defangv1.ServiceState_DEPLOYMENT_FAILED, "revision stopped", true
	case armappcontainersv3.RevisionRunningStateProcessing, armappcontainersv3.RevisionRunningStateUnknown, "":
		return defangv1.ServiceState_DEPLOYMENT_PENDING, "starting", false
	}
	// RunningState == Running — health is the final gate.
	switch s.HealthState {
	case armappcontainersv3.RevisionHealthStateHealthy:
		return defangv1.ServiceState_DEPLOYMENT_COMPLETED, "healthy", true
	case armappcontainersv3.RevisionHealthStateUnhealthy:
		// Keep polling — a brief Unhealthy window during startup is normal.
		return defangv1.ServiceState_DEPLOYMENT_PENDING, "unhealthy (warming)", false
	}
	// RevisionHealthStateNone (no probes configured) on a Running revision
	// is treated as success — there's nothing more to check.
	return defangv1.ServiceState_DEPLOYMENT_COMPLETED, "running", true
}

func pollBuilds(ctx context.Context, runs subscribeRunsClient, services []string, since time.Time, out chan<- *defangv1.SubscribeResponse, wg *sync.WaitGroup) {
	defer wg.Done()
	serviceSet := make(map[string]struct{}, len(services))
	for _, s := range services {
		serviceSet[s] = struct{}{}
	}

	// We only emit state transitions, not every poll. lastStatus is keyed
	// by RunID so two concurrent builds for the same service (unusual but
	// possible) don't suppress each other.
	lastStatus := make(map[string]armcontainerregistry.RunStatus)

	ticker := time.NewTicker(subscribePollInterval)
	defer ticker.Stop()

	for {
		infos, err := runs.ListRunsSince(ctx, since)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			term.Debugf("Subscribe: ACR runs error: %v", err)
		}
		for _, info := range infos {
			if _, want := serviceSet[info.Task]; !want {
				continue
			}
			if prev, ok := lastStatus[info.RunID]; ok && prev == info.Status {
				continue
			}
			lastStatus[info.RunID] = info.Status

			state, status := mapRunStatus(info.Status)
			if state == defangv1.ServiceState_NOT_SPECIFIED {
				continue
			}
			select {
			case out <- &defangv1.SubscribeResponse{
				Name:   info.Task,
				Status: status,
				State:  state,
			}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func mapRunStatus(s armcontainerregistry.RunStatus) (defangv1.ServiceState, string) {
	switch s {
	case armcontainerregistry.RunStatusQueued:
		return defangv1.ServiceState_BUILD_QUEUED, "queued"
	case armcontainerregistry.RunStatusStarted:
		return defangv1.ServiceState_BUILD_PROVISIONING, "started"
	case armcontainerregistry.RunStatusRunning:
		return defangv1.ServiceState_BUILD_RUNNING, "running"
	case armcontainerregistry.RunStatusSucceeded:
		return defangv1.ServiceState_BUILD_STOPPING, "build succeeded"
	case armcontainerregistry.RunStatusFailed,
		armcontainerregistry.RunStatusError,
		armcontainerregistry.RunStatusTimeout,
		armcontainerregistry.RunStatusCanceled:
		return defangv1.ServiceState_BUILD_FAILED, "build " + string(s)
	}
	return defangv1.ServiceState_NOT_SPECIFIED, ""
}
