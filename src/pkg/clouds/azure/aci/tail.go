package aci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/gorilla/websocket"
)

type LogEntry struct {
	Message string
	Stderr  bool
	Err     error
	Time    time.Time
}

func (c *ContainerInstance) Tail(ctx context.Context, groupName ContainerGroupName, containerName string) error {
	ch, err := c.StreamLogs(ctx, groupName, containerName)
	if err != nil {
		return err
	}

	for entry := range ch {
		if entry.Err != nil {
			return entry.Err
		}
		if entry.Stderr {
			fmt.Fprint(os.Stderr, entry.Message)
		} else {
			fmt.Print(entry.Message)
		}
	}
	return io.EOF
}

func (c *ContainerInstance) QueryLogs(ctx context.Context, groupName ContainerGroupName, containerName string) (string, error) {
	client, err := c.newContainerClient()
	if err != nil {
		return "", err
	}

	for {
		logResponse, err := client.ListLogs(ctx, c.resourceGroupName, *groupName, containerName, nil)
		if err != nil {
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && respErr.ErrorCode == "ContainerGroupDeploymentNotReady" {
				time.Sleep(time.Second) // Wait before retrying
				continue                // Retry if the deployment is not ready yet
			}
			return "", fmt.Errorf("failed to list logs: %w", err)
		}
		if logResponse.Logs.Content == nil {
			return "", io.EOF
		}
		return *logResponse.Logs.Content, nil
	}
}

const pollInterval = 2 * time.Second

// PollLogs streams container logs by periodically calling ListLogs and emitting new content.
// Unlike the websocket attach, this captures output produced before the call was made.
func (c *ContainerInstance) PollLogs(ctx context.Context, groupName ContainerGroupName, containerName string) (<-chan LogEntry, error) {
	ch := make(chan LogEntry)
	go func() {
		defer close(ch)
		var offset int
		poll := func() bool {
			content, err := c.QueryLogs(ctx, groupName, containerName)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					return false
				}
				// Container may still be starting; keep polling.
				return true
			}
			if len(content) <= offset {
				return true
			}
			newContent := content[offset:]
			offset = len(content)
			now := time.Now()
			for line := range strings.SplitSeq(newContent, "\n") {
				if line == "" {
					continue
				}
				select {
				case ch <- LogEntry{Message: line, Time: now}:
				case <-ctx.Done():
					return false
				}
			}
			return true
		}

		// Query immediately before the first tick so we don't miss early output.
		if !poll() {
			return
		}
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !poll() {
					return
				}
			}
		}
	}()
	return ch, nil
}

func (c *ContainerInstance) StreamLogs(ctx context.Context, groupName ContainerGroupName, containerName string) (<-chan LogEntry, error) {
	client, err := c.newContainerClient()
	if err != nil {
		return nil, err
	}

	var attachResponse armcontainerinstance.ContainersClientAttachResponse
	for {
		attachResponse, err = client.Attach(ctx, c.resourceGroupName, *groupName, containerName, nil)
		if err != nil {
			var respErr *azcore.ResponseError
			if errors.As(err, &respErr) && respErr.ErrorCode == "ContainerNotFound" {
				time.Sleep(time.Second) // Wait before retrying
				continue                // Retry if the container is not found yet
			}
			return nil, fmt.Errorf("failed to attach to container: %w", err)
		}
		break
	}

	header := http.Header{}
	header.Set("Authorization", *attachResponse.Password)
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, *attachResponse.WebSocketURI, header)
	defer resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to websocket (%s): %w", resp.Status, err)
	}

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		<-ctx.Done()
		_ = conn.Close() // unblock conn.ReadMessage
	}()

	ch := make(chan LogEntry)
	go func() {
		defer close(ch)
		defer cancel()

		for {
			_, logLine, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					select {
					case ch <- LogEntry{Err: err}:
					case <-ctx.Done():
					}
				}
				return
			}
			stdioFd := logLine[0]
			select {
			case ch <- LogEntry{Message: string(logLine[1:]), Stderr: stdioFd == 2}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
