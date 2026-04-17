package cd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const storageAccountPrefix = "defangcd"
const blobContainerName = "uploads"

// CreateResourceGroup creates or updates an Azure resource group with the given name.
func (d *Driver) CreateResourceGroup(ctx context.Context, name string) error {
	rgClient, err := d.newResourceGroupClient()
	if err != nil {
		return err
	}
	_, err = rgClient.CreateOrUpdate(ctx, name, armresources.ResourceGroup{
		Location: d.Location.Ptr(),
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to create resource group %q: %w", name, err)
	}
	return nil
}

// SetUpResourceGroup creates or updates the shared CD resource group (defang-cd-{location}).
func (d *Driver) SetUpResourceGroup(ctx context.Context) error {
	return d.CreateResourceGroup(ctx, d.resourceGroupName)
}

func (d *Driver) TearDown(ctx context.Context) error {
	rgClient, err := d.newResourceGroupClient()
	if err != nil {
		return err
	}
	deletePoller, err := rgClient.BeginDelete(ctx, d.resourceGroupName, nil)
	if err != nil {
		return fmt.Errorf("failed to delete resource group: %w", err)
	}
	_, err = deletePoller.PollUntilDone(ctx, nil)
	return err
}

func (d *Driver) getStorageAccount(ctx context.Context, accountsClient *armstorage.AccountsClient) (string, error) {
	if d.StorageAccount != "" {
		return d.StorageAccount, nil
	}

	if sa := os.Getenv("AZURE_STORAGE_ACCOUNT"); sa != "" {
		return sa, nil
	}

	for pager := accountsClient.NewListByResourceGroupPager(d.resourceGroupName, nil); pager.More(); {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list storage accounts: %w", err)
		}
		for _, account := range page.Value {
			if strings.HasPrefix(*account.Name, storageAccountPrefix) && *account.Location == d.Location.String() {
				return *account.Name, nil
			}
		}
	}
	return "", nil
}

func (d *Driver) SetUpStorageAccount(ctx context.Context) (string, error) {
	// Idempotency: skip if already set up.
	if d.StorageAccount != "" && d.BlobContainerName != "" {
		return d.StorageAccount, nil
	}

	accountsClient, err := d.NewStorageAccountsClient()
	if err != nil {
		return "", err
	}

	storageAccount, err := d.getStorageAccount(ctx, accountsClient)
	if err != nil {
		return "", err
	}

	if storageAccount == "" {
		storageAccount = storageAccountPrefix + pkg.RandomID()
		createPoller, err := accountsClient.BeginCreate(ctx, d.resourceGroupName, storageAccount, armstorage.AccountCreateParameters{
			Kind:     to.Ptr(armstorage.KindStorageV2),
			Location: d.Location.Ptr(),
			SKU:      &armstorage.SKU{Name: to.Ptr(armstorage.SKUNameStandardLRS)},
		}, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create storage account: %w", err)
		}
		_, err = createPoller.PollUntilDone(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("failed to poll storage account creation: %w", err)
		}
	}
	d.StorageAccount = storageAccount

	containerClient, err := d.NewBlobContainersClient()
	if err != nil {
		return "", fmt.Errorf("failed to create blob containers client: %w", err)
	}
	container, err := containerClient.Create(ctx, d.resourceGroupName, storageAccount, blobContainerName, armstorage.BlobContainer{}, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.ErrorCode == "ContainerAlreadyExists" {
			d.BlobContainerName = blobContainerName
		} else {
			return "", fmt.Errorf("failed to create blob container: %w", err)
		}
	} else {
		d.BlobContainerName = *container.Name
	}

	term.Infof("Using storage account %s and blob container %s", storageAccount, blobContainerName)

	return storageAccount, nil
}
