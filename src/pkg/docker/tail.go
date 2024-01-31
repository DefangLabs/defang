package docker

import (
	"context"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
)

func (d Docker) Tail(ctx context.Context, id ContainerID) error {
	rc, err := d.Client.ContainerLogs(ctx, *id, types.ContainerLogsOptions{
		Follow:     true,
		ShowStderr: true,
		ShowStdout: true,
	})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, rc)
	return err
}
