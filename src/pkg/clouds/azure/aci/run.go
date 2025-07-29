package aci

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
)

type ContainerGroupName = types.TaskID

// safeAppend is like append but avoids mutating any aliases of the slice.
func safeAppend[T any](slice []T, elems ...T) []T {
	return append(slice[:len(slice):len(slice)], elems...)
}

func (c *ContainerInstance) Run(ctx context.Context, env map[string]string, args ...string) (ContainerGroupName, error) {
	containerGroupClient, err := newContainerGroupClient()
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

	clone := *c.containerGroupProps
	for i, container := range clone.Containers {
		newProps := *container.Properties
		newProps.Command = safeAppend(newProps.Command, commandArgs...) // TODO: probably should only be done for the first container
		newProps.EnvironmentVariables = safeAppend(newProps.EnvironmentVariables, envVars...)
		clone.Containers[i] = &armcontainerinstance.Container{
			Name:       container.Name,
			Properties: &newProps,
		}
	}

	groupName := containerGroupPrefix + pkg.RandomID()
	group := armcontainerinstance.ContainerGroup{
		Name:       to.Ptr(groupName),
		Location:   c.Location.Ptr(),
		Properties: &clone,
	}
	_, err = containerGroupClient.BeginCreateOrUpdate(ctx, c.resourceGroupName, groupName, group, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container group: %w", err)
	}

	_, err = containerGroupClient.BeginStart(ctx, c.resourceGroupName, groupName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start container group: %w", err)
	}

	// createResponse.Done()
	// res, err := createResponse.PollUntilDone(ctx, nil)
	// if err != nil {
	// 	return nil, err
	// }
	return &groupName, nil
}
