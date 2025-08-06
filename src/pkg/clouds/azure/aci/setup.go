package aci

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/types"
)

const containerGroupPrefix = "defang-cd-"
const storageAccountPrefix = "defangcd"
const blobContainerName = "uploads"

func (c *ContainerInstance) SetUp(ctx context.Context, containers []types.Container) error {
	resourceGroupClient, err := c.newResourceGroupClient()
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
		if container.IsInit {
			properties := &armcontainerinstance.InitContainerPropertiesDefinition{
				Command: to.SliceOfPtrs(container.Command...),
				Image:   to.Ptr(container.Image),
			}
			c.containerGroupProps.InitContainers = append(c.containerGroupProps.InitContainers, &armcontainerinstance.InitContainerDefinition{
				Name:       to.Ptr(container.Name),
				Properties: properties,
			})
		} else {
			cpus := math.Max(0.01, container.Cpus)                                      // ensure minimum CPU is 0.01
			memoryInGB := math.Max(0.1, float64(container.Memory)/1024.0/1024.0/1024.0) // convert from B to GB, minimum 0.1
			properties := &armcontainerinstance.ContainerProperties{
				Command: to.SliceOfPtrs(container.Command...),
				Image:   to.Ptr(container.Image),
				Resources: &armcontainerinstance.ResourceRequirements{
					Requests: &armcontainerinstance.ResourceRequests{
						CPU:        to.Ptr(math.Round(100*cpus) / 100),      // round to 2 decimal places
						MemoryInGB: to.Ptr(math.Round(10*memoryInGB) * 0.1), // Round to 1 decimal place
					},
				},
			}
			c.containerGroupProps.Containers = append(c.containerGroupProps.Containers, &armcontainerinstance.Container{
				Name:       to.Ptr(container.Name),
				Properties: properties,
			})
		}
	}

	_, err = c.SetUpStorageAccount(ctx)
	if err != nil {
		return fmt.Errorf("failed to get storage account name: %w", err)
	}

	return nil
}

func (c *ContainerInstance) TearDown(ctx context.Context) error {
	resourceGroupClient, err := c.newResourceGroupClient()
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

	// TODO: delete storage account?
	return nil
}

func (c *ContainerInstance) getStorageAccount(ctx context.Context, accountsClient *armstorage.AccountsClient) (string, error) {
	if c.StorageAccount != "" {
		return c.StorageAccount, nil
	}

	for pager := accountsClient.NewListByResourceGroupPager(c.resourceGroupName, nil); pager.More(); {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list storage accounts: %w", err)
		}
		for _, account := range page.Value {
			if strings.HasPrefix(*account.Name, storageAccountPrefix) && *account.Location == c.Location.String() {
				return *account.Name, nil
			}
		}
	}
	return "", nil
}

func (c *ContainerInstance) SetUpStorageAccount(ctx context.Context) (string, error) {
	accountsClient, err := c.NewStorageAccountsClient()
	if err != nil {
		return "", err
	}

	storageAccount, err := c.getStorageAccount(ctx, accountsClient)
	if err != nil {
		return "", err
	}

	if storageAccount == "" {
		storageAccount = storageAccountPrefix + pkg.RandomID() // unique storage account name
		createResponse, err := accountsClient.BeginCreate(ctx, c.resourceGroupName, storageAccount, armstorage.AccountCreateParameters{
			Kind:     to.Ptr(armstorage.KindStorageV2),
			Location: c.Location.Ptr(),
			SKU:      &armstorage.SKU{Name: to.Ptr(armstorage.SKUNameStandardLRS)},
		}, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create storage account: %w", err)
		}

		_, err = createResponse.PollUntilDone(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("failed to poll storage account creation: %w", err)
		}
	}
	c.StorageAccount = storageAccount

	// Assign permissions
	// objectId, err := getCurrentUserObjectID(ctx)
	// if err != nil {
	// 	return "", fmt.Errorf("failed to get current user object ID: %w", err)
	// }
	// println("Current user object ID:", objectId)

	// if err := c.assignBlobDataContributor(ctx, objectId); err != nil {
	// 	return "", fmt.Errorf("failed to assign blob data contributor role: %w", err)
	// }

	// client, err := c.NewStorageAccountsClient()
	// if err != nil {
	// 	return "", fmt.Errorf("failed to create storage accounts client: %w", err)
	// }
	// lk, err := client.ListKeys(ctx, c.resourceGroupName, storageAccount, nil)
	// if err != nil {
	// 	return "", fmt.Errorf("failed to list storage account keys: %w", err)
	// }

	// ss, err := client.RegenerateKey(ctx, c.resourceGroupName, storageAccount, armstorage.RegenerateKeyParameters{
	// 	KeyName: to.Ptr("key1"),
	// }, nil)
	// if err != nil {
	// 	return "", fmt.Errorf("failed to regenerate storage account key: %w", err)
	// }

	containerClient, err := c.NewBlobContainersClient()
	if err != nil {
		return "", fmt.Errorf("failed to create storage client: %w", err)
	}
	container, err := containerClient.Create(ctx, c.resourceGroupName, storageAccount, blobContainerName, armstorage.BlobContainer{}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create blob container: %w", err)
	}
	c.BlobContainerName = *container.Name

	return storageAccount, nil
}
