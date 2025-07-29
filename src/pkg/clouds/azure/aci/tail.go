package aci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/gorilla/websocket"
)

type logEntry struct {
	Message string
	Stderr  bool
	Err     error
}

func (c *ContainerInstance) Tail(ctx context.Context, groupName ContainerGroupName) error {
	container := c.containerGroupProps.Containers[0]

	ch, err := c.StreamLogs(ctx, groupName, *container.Name)
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

func (c *ContainerInstance) StreamLogs(ctx context.Context, groupName ContainerGroupName, containerName string) (<-chan logEntry, error) {
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

	ch := make(chan logEntry)
	go func() {
		defer close(ch)
		defer cancel()

		for {
			_, logLine, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					select {
					case ch <- logEntry{Err: err}:
					case <-ctx.Done():
					}
				}
				return
			}
			stdioFd := logLine[0]
			select {
			case ch <- logEntry{Message: string(logLine[1:]), Stderr: stdioFd == 2}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
