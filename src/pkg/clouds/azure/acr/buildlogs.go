package acr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const buildPollInterval = 5 * time.Second

// BuildLogEntry is a log line from an ACR task run, annotated with the service being built.
type BuildLogEntry struct {
	Service string
	Line    string
	Err     error
}

// BuildLogWatcher polls for ACR task runs in a resource group and streams their logs.
type BuildLogWatcher struct {
	azure.Azure
	ResourceGroup string
}

// WatchBuildLogs discovers ACR registries in the resource group, polls for active task
// runs, and streams their build log output. The registry itself is created lazily by
// Pulumi during the CD run, so registry discovery is retried on every poll until one
// appears (or ctx is cancelled). The returned channel is closed when ctx is cancelled.
func (w *BuildLogWatcher) WatchBuildLogs(ctx context.Context) <-chan BuildLogEntry {
	out := make(chan BuildLogEntry)
	go func() {
		defer close(out)
		watchStart := time.Now().Add(-2 * time.Minute) // catch builds that started up to 2 min before tailing

		cred, err := w.NewCreds()
		if err != nil {
			term.Debugf("WatchBuildLogs: failed to get credentials: %v", err)
			return
		}

		regClient, err := armcontainerregistry.NewRegistriesClient(w.SubscriptionID, cred, nil)
		if err != nil {
			term.Debugf("WatchBuildLogs: failed to create registries client: %v", err)
			return
		}
		runsClient, err := armcontainerregistry.NewRunsClient(w.SubscriptionID, cred, nil)
		if err != nil {
			term.Debugf("WatchBuildLogs: failed to create runs client: %v", err)
			return
		}

		// Registry is discovered lazily — Pulumi creates it partway through the CD run,
		// so it is not guaranteed to exist when WatchBuildLogs starts.
		var registryName string
		// Track runs we're already streaming so we don't duplicate.
		streaming := map[string]struct{}{}
		// defaultService is learned from any run's OutputImages (populated after
		// completion) so we can label in-progress runs with the right service name.
		defaultService := ""

		findRegistry := func() string {
			pager := regClient.NewListByResourceGroupPager(w.ResourceGroup, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					term.Debugf("WatchBuildLogs: failed to list registries: %v", err)
					return ""
				}
				for _, reg := range page.Value {
					if reg.Name != nil {
						return *reg.Name
					}
				}
			}
			return ""
		}

		poll := func() {
			if registryName == "" {
				registryName = findRegistry()
				if registryName == "" {
					return // no registry yet; retry next tick
				}
				term.Debugf("WatchBuildLogs: found registry %q in %q", registryName, w.ResourceGroup)
			}

			// List the most recent runs (no status filter) so we catch builds that
			// started and finished between polls.
			top := int32(10)
			pager := runsClient.NewListPager(w.ResourceGroup, registryName, &armcontainerregistry.RunsClientListOptions{
				Top: &top,
			})
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					term.Debugf("WatchBuildLogs: failed to list runs: %v", err)
					return
				}
				for _, run := range page.Value {
					if run.Properties == nil || run.Properties.RunID == nil {
						continue
					}
					// Learn service name from any completed run's OutputImages.
					if imgs := run.Properties.OutputImages; len(imgs) > 0 && imgs[0].Repository != nil {
						defaultService = *imgs[0].Repository
					}
					runID := *run.Properties.RunID
					if _, ok := streaming[runID]; ok {
						continue
					}
					// Only stream runs that started after the watcher was created.
					if run.Properties.CreateTime != nil && run.Properties.CreateTime.Before(watchStart) {
						continue
					}
					streaming[runID] = struct{}{}
					service := defaultService
					if service == "" {
						service = runID
					}
					term.Debugf("WatchBuildLogs: streaming run %s (service %s)", runID, service)
					go w.streamRunLog(ctx, runsClient, w.ResourceGroup, registryName, runID, service, out)
				}
			}
		}

		poll()
		ticker := time.NewTicker(buildPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poll()
			}
		}
	}()
	return out
}

// streamRunLog polls GetLogSasURL for a run and streams new log content as it grows.
func (w *BuildLogWatcher) streamRunLog(
	ctx context.Context,
	runsClient *armcontainerregistry.RunsClient,
	rgName, registryName, runID, service string,
	out chan<- BuildLogEntry,
) {
	var lastLen int

	for {
		// Get the log SAS URL (regenerated each call but points to the same growing blob).
		logResp, err := runsClient.GetLogSasURL(ctx, rgName, registryName, runID, nil)
		if err != nil {
			term.Debugf("streamRunLog %s: GetLogSasURL error: %v", runID, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(buildPollInterval):
			}
			continue
		}
		if logResp.LogLink == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(buildPollInterval):
			}
			continue
		}

		// Fetch the full log content and emit only new lines.
		content, err := fetchLogContent(ctx, *logResp.LogLink)
		if err != nil {
			term.Debugf("streamRunLog %s: fetch error: %v", runID, err)
		} else if len(content) > lastLen {
			newContent := content[lastLen:]
			lastLen = len(content)
			for _, line := range strings.Split(strings.TrimRight(newContent, "\n"), "\n") {
				if line == "" {
					continue
				}
				select {
				case out <- BuildLogEntry{Service: service, Line: line}:
				case <-ctx.Done():
					return
				}
			}
		}

		// Check if run is still active.
		runResp, err := runsClient.Get(ctx, rgName, registryName, runID, nil)
		if err == nil && runResp.Properties != nil && runResp.Properties.Status != nil {
			switch *runResp.Properties.Status {
			case armcontainerregistry.RunStatusSucceeded,
				armcontainerregistry.RunStatusFailed,
				armcontainerregistry.RunStatusError,
				armcontainerregistry.RunStatusCanceled,
				armcontainerregistry.RunStatusTimeout:
				// Do one final fetch to capture any remaining log lines.
				if logResp.LogLink != nil {
					finalContent, err := fetchLogContent(ctx, *logResp.LogLink)
					if err == nil && len(finalContent) > lastLen {
						for _, line := range strings.Split(strings.TrimRight(finalContent[lastLen:], "\n"), "\n") {
							if line == "" {
								continue
							}
							select {
							case out <- BuildLogEntry{Service: service, Line: line}:
							case <-ctx.Done():
								return
							}
						}
					}
				}
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(buildPollInterval):
		}
	}
}

func fetchLogContent(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB max
	if err != nil {
		return "", err
	}
	return string(body), nil
}
