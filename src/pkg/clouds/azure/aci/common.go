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
	storageAccount      string
}

func NewContainerInstance(resourceGroupName string, location azure.Location) *ContainerInstance {
	if location == "" {
		location = azure.Location(os.Getenv("AZURE_LOCATION"))
	}
	return &ContainerInstance{
		Azure:             azure.Azure{Location: location},
		resourceGroupName: resourceGroupName, // TODO: append location?
		storageAccount:    os.Getenv("DEFANG_CD_BUCKET"),
	}
}
func newContainerGroupClient() (*armcontainerinstance.ContainerGroupsClient, error) {
	subscriptionID, cred, err := azure.NewCreds()
	if err != nil {
		return nil, err
	}

	clientFactory, err := armcontainerinstance.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container group client: %w", err)
	}
	return clientFactory.NewContainerGroupsClient(), nil
}

func newContainerClient() (*armcontainerinstance.ContainersClient, error) {
	subscriptionID, cred, err := azure.NewCreds()
	if err != nil {
		return nil, err
	}

	clientFactory, err := armcontainerinstance.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create container client: %w", err)
	}
	return clientFactory.NewContainersClient(), nil
}

func newResourceGroupClient() (*armresources.ResourceGroupsClient, error) {
	subscriptionID, cred, err := azure.NewCreds()
	if err != nil {
		return nil, err
	}

	resourcesClientFactory, err := armresources.NewClientFactory(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group client: %w", err)
	}
	return resourcesClientFactory.NewResourceGroupsClient(), nil
}
