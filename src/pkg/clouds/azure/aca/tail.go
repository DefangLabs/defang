package aca

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const watchInterval = 5 * time.Second

type LogEntry struct {
	Message string
	Err     error
}

// ServiceLogEntry is a LogEntry annotated with the Container App name it came from.
type ServiceLogEntry struct {
	AppName string
	LogEntry
}

// WatchLogs polls the resource group for Container Apps every watchInterval and streams
// logs from each one as soon as it is discovered. New apps that appear after the initial
// poll are picked up automatically.
func (c *ContainerApp) WatchLogs(ctx context.Context) <-chan ServiceLogEntry {
	out := make(chan ServiceLogEntry)
	go func() {
		// streaming tracks apps that currently have a live tail goroutine. An
		// app is re-added to the map on the next poll once its goroutine exits
		// (so replicas that roll or streams that drop mid-run are retried).
		var mu sync.Mutex
		streaming := map[string]struct{}{}

		// senders tracks inner goroutines that send on `out` (per-app tailers
		// and pollErr). We must wait for all of them before closing `out`,
		// otherwise a `case out <- …` racing with our close panics with
		// "send on closed channel".
		var senders sync.WaitGroup
		defer func() {
			senders.Wait()
			close(out)
		}()

		startTailing := func(appName string) {
			senders.Add(1)
			go func() {
				defer senders.Done()
				defer func() {
					mu.Lock()
					delete(streaming, appName)
					mu.Unlock()
				}()
				appCh, err := c.StreamLogs(ctx, appName, "", "", "", true)
				if err != nil {
					select {
					case out <- ServiceLogEntry{AppName: appName, LogEntry: LogEntry{Err: err}}:
					case <-ctx.Done():
					}
					return
				}
				for entry := range appCh {
					select {
					case out <- ServiceLogEntry{AppName: appName, LogEntry: entry}:
					case <-ctx.Done():
						return
					}
				}
			}()
		}

		sendErr := func(err error) {
			select {
			case out <- ServiceLogEntry{LogEntry: LogEntry{Err: err}}:
			case <-ctx.Done():
			}
		}

		poll := func() {
			client, err := c.newContainerAppsClient()
			if err != nil {
				sendErr(fmt.Errorf("WatchLogs: create container apps client: %w", err))
				return
			}
			pager := client.NewListByResourceGroupPager(c.ResourceGroup, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					sendErr(fmt.Errorf("WatchLogs: list container apps: %w", err))
					return
				}
				for _, app := range page.Value {
					if app == nil || app.Name == nil {
						continue
					}
					name := *app.Name
					mu.Lock()
					if _, active := streaming[name]; active {
						mu.Unlock()
						continue
					}
					streaming[name] = struct{}{}
					mu.Unlock()
					startTailing(name)
				}
			}
		}

		poll()
		ticker := time.NewTicker(watchInterval)
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

// StreamLogs streams real-time logs from a Container App container via Server-Sent Events.
// revision, replica, and container may be empty; they will be resolved to the latest active
// revision, first replica, and first container automatically.
// When follow is false, the stream ends when there are no more buffered log lines.
func (c *ContainerApp) StreamLogs(ctx context.Context, appName, revision, replica, container string, follow bool) (<-chan LogEntry, error) {
	var err error
	revision, replica, container, err = c.ResolveLogTarget(ctx, appName, revision, replica, container)
	if err != nil {
		return nil, err
	}

	baseURL, err := c.getEventStreamBase(ctx, appName)
	if err != nil {
		return nil, err
	}

	authToken, err := c.getAuthToken(ctx, appName)
	if err != nil {
		return nil, err
	}

	streamURL := fmt.Sprintf(
		"%s/subscriptions/%s/resourceGroups/%s/containerApps/%s/revisions/%s/replicas/%s/containers/%s/logstream",
		baseURL, c.SubscriptionID, c.ResourceGroup, appName, revision, replica, container,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+authToken)

	q := req.URL.Query()
	q.Set("follow", strconv.FormatBool(follow))
	q.Set("output", "text")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req) // nolint resp.Body is closed by the goroutine below via defer resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("log stream: HTTP %s", resp.Status)
	}

	ch := make(chan LogEntry)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			select {
			case ch <- LogEntry{Message: line}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case ch <- LogEntry{Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}
