package aci

import (
	"context"
	"errors"
)

func (c *ContainerInstance) Stop(ctx context.Context, groupName ContainerGroupName) error {
	if groupName == nil {
		return errors.New("container group name is nil")
	}
	containerGroupClient, err := c.newContainerGroupClient()
	if err != nil {
		return err
	}

	_, err = containerGroupClient.Stop(ctx, c.resourceGroupName, *groupName, nil)
	if err != nil {
		return err
	}
	return nil
}
