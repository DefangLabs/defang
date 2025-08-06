package aci

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerinstance/armcontainerinstance/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
)

type ContainerInstance struct {
	azure.Azure
	containerGroupProps *armcontainerinstance.ContainerGroupPropertiesProperties
	resourceGroupName   string
	StorageAccount      string
	BlobContainerName   string
}

func NewContainerInstance(resourceGroupName string, location azure.Location) *ContainerInstance {
	if location == "" {
		location = azure.Location(os.Getenv("AZURE_LOCATION"))
	}
	return &ContainerInstance{
		Azure: azure.Azure{
			Location:       location,
			SubscriptionID: os.Getenv("AZURE_SUBSCRIPTION_ID"),
		},
		resourceGroupName: resourceGroupName, // TODO: append location?
		StorageAccount:    os.Getenv("DEFANG_CD_BUCKET"),
	}
}

func (c ContainerInstance) newContainerGroupClient() (*armcontainerinstance.ContainerGroupsClient, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return nil, err
	}

	clientFactory, err := armcontainerinstance.NewClientFactory(c.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container group client: %w", err)
	}
	return clientFactory.NewContainerGroupsClient(), nil
}

func (c ContainerInstance) newContainerClient() (*armcontainerinstance.ContainersClient, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return nil, err
	}

	clientFactory, err := armcontainerinstance.NewClientFactory(c.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container client: %w", err)
	}
	return clientFactory.NewContainersClient(), nil
}

func (c ContainerInstance) newResourceGroupClient() (*armresources.ResourceGroupsClient, error) {
	cred, err := c.NewCreds()
	if err != nil {
		return nil, err
	}

	resourcesClientFactory, err := armresources.NewClientFactory(c.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group client: %w", err)
	}
	return resourcesClientFactory.NewResourceGroupsClient(), nil
}
