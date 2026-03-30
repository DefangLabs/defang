package aci

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const containerGroupPrefix = "defang-cd-"
const storageAccountPrefix = "defangcd"
const blobContainerName = "uploads"

func (c *ContainerInstance) SetUpResourceGroup(ctx context.Context) error {
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

	if sa := os.Getenv("AZURE_STORAGE_ACCOUNT"); sa != "" {
		return sa, nil
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

	term.Infof("Using storage account %s and blob container %s", storageAccount, blobContainerName)

	return storageAccount, nil
}
