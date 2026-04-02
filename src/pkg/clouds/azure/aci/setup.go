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

// Well-known Azure built-in role definition IDs.
const (
	storageBlobDataContributorRoleID = "ba92f5b4-2d11-453d-a403-e96b0029c9fe"
	contributorRoleID                = "b24988ac-6180-42a0-ab88-20f7382dd24c"
	userAccessAdministratorRoleID    = "18d7d88d-d35e-4fb5-a5c3-7773c20a72d9"
)

// assignRole assigns a built-in role to the given principal at the given scope.
// It silently ignores RoleAssignmentExists errors (idempotent).
func assignRole(ctx context.Context, raClient *armauthorization.RoleAssignmentsClient, subscriptionID, scope, roleDefID, principalID string) error {
	fullRoleDefID := fmt.Sprintf("/subscriptions/%s/providers/Microsoft.Authorization/roleDefinitions/%s", subscriptionID, roleDefID)
	_, err := raClient.Create(ctx, scope, uuid.NewString(), armauthorization.RoleAssignmentCreateParameters{
		Properties: &armauthorization.RoleAssignmentProperties{
			PrincipalID:      to.Ptr(principalID),
			RoleDefinitionID: to.Ptr(fullRoleDefID),
			PrincipalType:    to.Ptr(armauthorization.PrincipalTypeServicePrincipal),
		},
	}, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if !errors.As(err, &respErr) || respErr.ErrorCode != "RoleAssignmentExists" {
			return err
		}
	}
	return nil
}

// SetUpManagedIdentity creates (or retrieves) a user-assigned managed identity, assigns it
// Contributor on the subscription (for Pulumi to provision resources), and Storage Blob Data
// Contributor on the storage account (for Pulumi state and payload access).
// The identity resource ID is stored in c.ManagedIdentityID.
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

	raClient, err := armauthorization.NewRoleAssignmentsClient(c.SubscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("failed to create role assignments client: %w", err)
	}

	// Contributor + User Access Administrator on the subscription so Pulumi can provision any
	// Azure resource and create role assignments (e.g. ACR pull role for Container Apps).
	subscriptionScope := "/subscriptions/" + c.SubscriptionID
	if err := assignRole(ctx, raClient, c.SubscriptionID, subscriptionScope, contributorRoleID, principalID); err != nil {
		return fmt.Errorf("failed to assign Contributor role: %w", err)
	}
	if err := assignRole(ctx, raClient, c.SubscriptionID, subscriptionScope, userAccessAdministratorRoleID, principalID); err != nil {
		return fmt.Errorf("failed to assign User Access Administrator role: %w", err)
	}

	// Storage Blob Data Contributor on the storage account for Pulumi state and payload access.
	storageScope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		c.SubscriptionID, c.resourceGroupName, c.StorageAccount,
	)
	if err := assignRole(ctx, raClient, c.SubscriptionID, storageScope, storageBlobDataContributorRoleID, principalID); err != nil {
		return fmt.Errorf("failed to assign Storage Blob Data Contributor role: %w", err)
	}

	return nil
}
