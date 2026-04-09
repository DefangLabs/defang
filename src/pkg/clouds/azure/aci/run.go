package aci

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
)

const cdContainerGroupName = "defang-cd"

// containerGroupIdentity returns a user-assigned identity block if one has been configured,
// or nil if no managed identity is set up yet.
func (c *ContainerInstance) containerGroupIdentity() *armcontainerinstance.ContainerGroupIdentity {
	if c.ManagedIdentityID == "" {
		return nil
	}
	return &armcontainerinstance.ContainerGroupIdentity{
		Type: to.Ptr(armcontainerinstance.ResourceIdentityTypeUserAssigned),
		UserAssignedIdentities: map[string]*armcontainerinstance.UserAssignedIdentities{
			c.ManagedIdentityID: {},
		},
	}
}

type ContainerGroupName = *string

func (c *ContainerInstance) Run(ctx context.Context, containers []*armcontainerinstance.Container, env map[string]string, args ...string) (ContainerGroupName, error) {
	containerGroupClient, err := c.newContainerGroupClient()
	if err != nil {
		return nil, err
	}

	commandArgs := to.SliceOfPtrs(args...)
	var envVars []*armcontainerinstance.EnvironmentVariable
	for key, value := range env {
		envVars = append(envVars, &armcontainerinstance.EnvironmentVariable{
			Name:  to.Ptr(key),
			Value: to.Ptr(value),
		})
	}

	clone := *c.ContainerGroupProps
	if len(containers) == 0 {
		containers = c.ContainerGroupProps.Containers
	}
	clone.Containers = make([]*armcontainerinstance.Container, len(containers))
	for i, container := range containers {
		if container == nil || container.Properties == nil {
			return nil, fmt.Errorf("container %d has nil properties", i)
		}
		newProps := *container.Properties
		if i == 0 {
			newProps.Command = append(newProps.Command, commandArgs...)
		}
		newProps.EnvironmentVariables = append(newProps.EnvironmentVariables, envVars...)
		clone.Containers[i] = &armcontainerinstance.Container{
			Name:       container.Name,
			Properties: &newProps,
		}
	}

	groupName := cdContainerGroupName
	group := armcontainerinstance.ContainerGroup{
		Name:       to.Ptr(groupName),
		Location:   c.Location.Ptr(),
		Identity:   c.containerGroupIdentity(),
		Properties: &clone,
	}
	createPoller, err := containerGroupClient.BeginCreateOrUpdate(ctx, c.resourceGroupName, groupName, group, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container group: %w", err)
	}
	if _, err := createPoller.PollUntilDone(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to complete container group creation: %w", err)
	}

	startPoller, err := containerGroupClient.BeginStart(ctx, c.resourceGroupName, groupName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start container group: %w", err)
	}
	if _, err := startPoller.PollUntilDone(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to complete container group start: %w", err)
	}

	return &groupName, nil
}
