package acr

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

// RunInfo summarizes one ACR Task run in a form convenient for the CLI's
// build-state machine.
type RunInfo struct {
	RunID      string
	Task       string // the persistent task name; matches the defang service name (see pulumi-defang createACRTask)
	Status     armcontainerregistry.RunStatus
	CreateTime time.Time
}

// RunsLister polls a resource group's ACR registries for Task runs. It is a
// sibling of BuildLogWatcher: BuildLogWatcher streams log lines, RunsLister
// emits run-status transitions used by Subscribe to drive BUILD_* state.
type RunsLister struct {
	azure.Azure
	ResourceGroup string

	regClient    *armcontainerregistry.RegistriesClient
	runsClient   *armcontainerregistry.RunsClient
	registryName string // cached after first successful discovery
}

func (r *RunsLister) ensureClients() error {
	if r.runsClient != nil {
		return nil
	}
	cred, err := r.NewCreds()
	if err != nil {
		return err
	}
	regClient, err := armcontainerregistry.NewRegistriesClient(r.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("create registries client: %w", err)
	}
	runsClient, err := armcontainerregistry.NewRunsClient(r.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("create runs client: %w", err)
	}
	r.regClient = regClient
	r.runsClient = runsClient
	return nil
}

// findRegistry returns the first registry in the resource group, or ""
// if Pulumi hasn't created one yet. Cached after first success.
func (r *RunsLister) findRegistry(ctx context.Context) (string, error) {
	if r.registryName != "" {
		return r.registryName, nil
	}
	if err := r.ensureClients(); err != nil {
		return "", err
	}
	pager := r.regClient.NewListByResourceGroupPager(r.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("list registries in %s: %w", r.ResourceGroup, err)
		}
		for _, reg := range page.Value {
			if reg.Name != nil {
				r.registryName = *reg.Name
				return r.registryName, nil
			}
		}
	}
	return "", nil // no registry yet (Pulumi creates it mid-deploy)
}

// ListRunsSince returns the most-recent ACR Task runs whose CreateTime is at
// or after `since`. Returns nil with no error when no registry exists yet.
func (r *RunsLister) ListRunsSince(ctx context.Context, since time.Time) ([]RunInfo, error) {
	if err := r.ensureClients(); err != nil {
		return nil, err
	}
	registry, err := r.findRegistry(ctx)
	if err != nil {
		return nil, err
	}
	if registry == "" {
		return nil, nil
	}

	top := int32(50)
	pager := r.runsClient.NewListPager(r.ResourceGroup, registry, &armcontainerregistry.RunsClientListOptions{
		Top: &top,
	})

	var out []RunInfo
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return out, fmt.Errorf("list runs in %s/%s: %w", r.ResourceGroup, registry, err)
		}
		for _, run := range page.Value {
			if run == nil || run.Properties == nil || run.Properties.RunID == nil {
				continue
			}
			create := time.Time{}
			if run.Properties.CreateTime != nil {
				create = *run.Properties.CreateTime
			}
			if !since.IsZero() && create.Before(since) {
				// Runs come back in descending create-time order, so once we
				// hit one older than `since` we can stop scanning this page.
				return out, nil
			}
			info := RunInfo{
				RunID:      *run.Properties.RunID,
				CreateTime: create,
			}
			if run.Properties.Task != nil {
				// `Task` on a freshly-scheduled run is a full resource ID; the
				// task name (= defang service name) is the last segment.
				info.Task = path.Base(*run.Properties.Task)
			}
			if run.Properties.Status != nil {
				info.Status = *run.Properties.Status
			}
			out = append(out, info)
		}
	}
	return out, nil
}

// IsTerminal reports whether an ACR run status is final.
func IsTerminal(s armcontainerregistry.RunStatus) bool {
	switch s {
	case armcontainerregistry.RunStatusSucceeded,
		armcontainerregistry.RunStatusFailed,
		armcontainerregistry.RunStatusError,
		armcontainerregistry.RunStatusCanceled,
		armcontainerregistry.RunStatusTimeout:
		return true
	}
	return false
}

// IsSuccess reports whether an ACR run completed successfully.
func IsSuccess(s armcontainerregistry.RunStatus) bool {
	return s == armcontainerregistry.RunStatusSucceeded
}
