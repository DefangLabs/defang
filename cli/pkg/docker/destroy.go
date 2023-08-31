package docker

import "context"

func (d Docker) Destroy(ctx context.Context) error {
	// return d.Client.ContainerRemove(ctx, d.containerID, types.ContainerRemoveOptions{}) TODO:
	return nil
}
