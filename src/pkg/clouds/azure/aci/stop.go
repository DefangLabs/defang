package aci

import (
	"context"
)

func (c *ContainerInstance) Stop(ctx context.Context, groupName ContainerGroupName) error {
	containerGroupClient, err := newContainerGroupClient()
	if err != nil {
		return err
	}

	_, err = containerGroupClient.Stop(ctx, c.resourceGroupName, *groupName, nil)
	if err != nil {
		return err
	}
	return nil
}
