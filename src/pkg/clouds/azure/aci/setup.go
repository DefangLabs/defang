package aci

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const containerGroupName = "defang-cd"

func (c *ContainerInstance) SetUp(ctx context.Context, containers []types.Container) error {
	resourceGroupClient, err := newResourceGroupClient()
	if err != nil {
		return err
	}
	_, err = resourceGroupClient.CreateOrUpdate(ctx, c.resourceGroupName, armresources.ResourceGroup{
		Location: c.Location.Ptr(),
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create resource group: %w", err)
	}

	c.containerGroupProps = &armcontainerinstance.ContainerGroupPropertiesProperties{
		OSType: to.Ptr(armcontainerinstance.OperatingSystemTypesLinux), // TODO: from Platform
		// Priority:      to.Ptr(armcontainerinstance.ContainerGroupPrioritySpot),
		RestartPolicy: to.Ptr(armcontainerinstance.ContainerGroupRestartPolicyNever),
	}
	if username := os.Getenv("DOCKERHUB_USERNAME"); username != "" {
		c.containerGroupProps.ImageRegistryCredentials = append(c.containerGroupProps.ImageRegistryCredentials, &armcontainerinstance.ImageRegistryCredential{
			Server:   to.Ptr("index.docker.io"),
			Username: to.Ptr(username),
			Password: to.Ptr(pkg.Getenv("DOCKERHUB_TOKEN", os.Getenv("DOCKERHUB_PASSWORD"))),
		})
	}

	for _, container := range containers {
		cpus := math.Max(0.01, container.Cpus)                                      // ensure minimum CPU is 0.01
		memoryInGB := math.Max(0.1, float64(container.Memory)/1024.0/1024.0/1024.0) // convert from B to GB
		properties := &armcontainerinstance.ContainerProperties{
			Image: to.Ptr(container.Image),
			Resources: &armcontainerinstance.ResourceRequirements{
				Requests: &armcontainerinstance.ResourceRequests{
					CPU:        to.Ptr(math.Round(100*cpus) / 100),      // round to 2 decimal places
					MemoryInGB: to.Ptr(math.Round(10*memoryInGB) * 0.1), // Round to 1 decimal place
				},
			},
		}
		for _, command := range container.Command {
			properties.Command = append(properties.Command, to.Ptr(command))
		}
		c.containerGroupProps.Containers = append(c.containerGroupProps.Containers, &armcontainerinstance.Container{
			Name:       to.Ptr(container.Name),
			Properties: properties,
		})
	}

	// newContainerGroupClient, err := newContainerGroupClient()
	// if err != nil {
	// 	return err
	// }
	// deleteResponse, err := newContainerGroupClient.BeginDelete(ctx, c.resourceGroupName, containerGroupName, nil)
	// if err != nil {
	// 	return err
	// }
	// _, err = deleteResponse.PollUntilDone(ctx, nil)
	// if err != nil {
	// 	return err
	// }
	return nil
}

func (c *ContainerInstance) TearDown(ctx context.Context) error {
	resourceGroupClient, err := newResourceGroupClient()
	if err != nil {
		return err
	}
	deleteResponse, err := resourceGroupClient.BeginDelete(ctx, c.resourceGroupName, nil)
	if err != nil {
		return fmt.Errorf("failed to delete resource group: %w", err)
	}
	_, err = deleteResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}
	return nil
}
