package docker

import (
	"context"

	"github.com/docker/docker/api/types/container"
)

func (d Docker) Stop(ctx context.Context, id ContainerID) error {
	return d.ContainerStop(ctx, *id, container.StopOptions{})
}
