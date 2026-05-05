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
	"github.com/DefangLabs/defang/src/pkg/clouds/azure"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const storageAccountPrefix = "defangcd"

// Container names used in the CD storage account. Keep them DNS-safe:
// 3–63 chars, lowercase alphanumeric + hyphens (no leading/trailing hyphen).
const (
	// pulumiContainerName is the dedicated Pulumi state backend container.
	pulumiContainerName = "pulumi"
	// projectsContainerName holds {project}/{stack}/project.pb audit blobs
	// written by the CD task after each deploy.
	projectsContainerName = "projects"
)

// CreateResourceGroup creates or updates an Azure resource group with the given name.
func (d *Driver) CreateResourceGroup(ctx context.Context, name string) error {
	defer term.Timing()()
	term.Debugf("Creating or updating resource group %q in %q", name, d.Location)
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
	defer term.Timing()()
	return d.CreateResourceGroup(ctx, d.resourceGroupName)
}

func (d *Driver) TearDown(ctx context.Context) error {
	defer term.Timing()()
	rgClient, err := d.newResourceGroupClient()
	if err != nil {
		return err
	}
	deletePoller, err := rgClient.BeginDelete(ctx, d.resourceGroupName, nil)
	if err != nil {
		return fmt.Errorf("failed to delete resource group: %w", err)
	}
	_, err = deletePoller.PollUntilDone(ctx, azure.PollOptions)
	return err
}

func (d *Driver) getStorageAccount(ctx context.Context, accountsClient *armstorage.AccountsClient) (string, error) {
	defer term.Timing()()
	if d.StorageAccount != "" {
		return d.StorageAccount, nil
	}

	if sa := os.Getenv("AZURE_STORAGE_ACCOUNT"); sa != "" {
		return sa, nil
	}

	term.Debugf("getStorageAccount from resource group %q", d.resourceGroupName)
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

// resolveBlobContainer picks the blob container in use on the given storage
// account. It prefers the legacy `pulumi` container if it exists (carry-over
// from older installs where Pulumi state lived in its own container);
// otherwise it returns the canonical `projects` container, which now holds
// both Pulumi state and project.pb audit blobs. When create is true, the
// `projects` container is created if missing (idempotent — "already exists"
// is treated as success).
func (d *Driver) resolveBlobContainer(ctx context.Context, storageAccount string, create bool) (string, error) {
	containerClient, err := d.NewBlobContainersClient()
	if err != nil {
		return "", err
	}
	name := pulumiContainerName
	if _, err := containerClient.Get(ctx, d.resourceGroupName, storageAccount, name, nil); err != nil {
		var respErr *azcore.ResponseError
		if !errors.As(err, &respErr) || respErr.StatusCode != 404 {
			return "", fmt.Errorf("failed to look up blob container %q: %w", name, err)
		}
		name = projectsContainerName
		if create {
			term.Debugf("Create blob container %q", name)
			if _, err := containerClient.Create(ctx, d.resourceGroupName, storageAccount, name, armstorage.BlobContainer{}, nil); err != nil {
				var respErr *azcore.ResponseError
				if !errors.As(err, &respErr) || respErr.ErrorCode != "ContainerAlreadyExists" {
					return "", fmt.Errorf("failed to create blob container %q: %w", name, err)
				}
			}
		}
	}
	return name, nil
}

// FindStorageAccount is a read-only variant of SetUpStorageAccount: it locates
// the defang CD storage account (and remembers its container) without
// creating anything. Returns ("", nil) when the storage account doesn't exist
// yet — typical for a subscription where defang has never been deployed. On
// success, d.StorageAccount and d.BlobContainerName are populated for
// subsequent DownloadBlob / IterateBlobs calls. The `projects` container is
// not verified — downstream blob ops will return 404 if it hasn't been
// created yet, which callers already handle.
func (d *Driver) FindStorageAccount(ctx context.Context) (string, error) {
	defer term.Timing()()
	if d.StorageAccount != "" && d.BlobContainerName != "" {
		return d.StorageAccount, nil
	}
	accountsClient, err := d.NewStorageAccountsClient()
	if err != nil {
		return "", err
	}
	storageAccount, err := d.getStorageAccount(ctx, accountsClient)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			return "", nil // resource group doesn't exist yet
		}
		return "", err
	}
	if storageAccount == "" {
		return "", nil
	}
	name, err := d.resolveBlobContainer(ctx, storageAccount, false)
	if err != nil {
		return "", err
	}
	d.StorageAccount = storageAccount
	d.BlobContainerName = name
	return storageAccount, nil
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
		_, err = createPoller.PollUntilDone(ctx, azure.PollOptions)
		if err != nil {
			return "", fmt.Errorf("failed to poll storage account creation: %w", err)
		}
	}
	d.StorageAccount = storageAccount

	name, err := d.resolveBlobContainer(ctx, storageAccount, true)
	if err != nil {
		return "", err
	}
	d.BlobContainerName = name

	term.Infof("Using storage account %s container %s", storageAccount, name)

	return storageAccount, nil
}
