package aci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/google/uuid"
)

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

const managedIdentityName = "defang-cd-identity"

// storageBlobDataContributorRoleID is the well-known Azure role definition ID for Storage Blob Data Contributor.
const storageBlobDataContributorRoleID = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"

// SetUpManagedIdentity creates (or retrieves) a user-assigned managed identity and assigns it
// Storage Blob Data Reader on the storage account. The identity resource ID is stored in
// c.ManagedIdentityID.
func (c *ContainerInstance) SetUpManagedIdentity(ctx context.Context) error {
	cred, err := c.NewCreds()
	if err != nil {
		return err
	}

	msiClient, err := armmsi.NewUserAssignedIdentitiesClient(c.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("failed to create MSI client: %w", err)
	}

	identity, err := msiClient.CreateOrUpdate(ctx, c.resourceGroupName, managedIdentityName, armmsi.Identity{
		Location: c.Location.Ptr(),
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create managed identity: %w", err)
	}
	c.ManagedIdentityID = *identity.ID
	principalID := *identity.Properties.PrincipalID

	// Assign Storage Blob Data Reader on the storage account.
	storageAccountScope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		c.SubscriptionID, c.resourceGroupName, c.StorageAccount,
	)
	roleDefID := fmt.Sprintf(
		"/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s",
		c.SubscriptionID, storageBlobDataContributorRoleID,
	)

	raClient, err := armauthorization.NewRoleAssignmentsClient(c.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("failed to create role assignments client: %w", err)
	}

	_, err = raClient.Create(ctx, storageAccountScope, uuid.NewString(), armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      to.Ptr(principalID),
			RoleDefinitionID: to.Ptr(roleDefID),
			PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
		},
	}, nil)
	if err != nil {
		// RoleAssignmentExists (code 409) is benign — the assignment is already in place.
		var respErr *azcore.ResponseError
		if !errors.As(err, &respErr) || respErr.ErrorCode != "RoleAssignmentExists" {
			return fmt.Errorf("failed to assign Storage Blob Data Reader role: %w", err)
		}
	}

	return nil
}
