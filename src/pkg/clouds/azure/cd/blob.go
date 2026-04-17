package cd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

const maxBlobDownloadSize = 32 * 1024 * 1024 // 32 MiB

// BlobItem represents a blob in the storage account container.
type BlobItem struct {
	name string
	size int64
}

func (b BlobItem) Name() string { return b.name }
func (b BlobItem) Size() int64  { return b.size }

func (d *Driver) newSharedKeyCredential(ctx context.Context) (*azblob.SharedKeyCredential, error) {
	storageKey := os.Getenv("AZURE_STORAGE_KEY")
	if storageKey == "" {
		accountsClient, err := d.NewStorageAccountsClient()
		if err != nil {
			return nil, err
		}
		keys, err := accountsClient.ListKeys(ctx, d.resourceGroupName, d.StorageAccount, nil)
		if err != nil {
			return nil, err
		}
		if len(keys.Keys) == 0 || keys.Keys[0].Value == nil {
			return nil, errors.New("no storage account keys returned")
		}
		storageKey = *keys.Keys[0].Value
	}
	return azblob.NewSharedKeyCredential(d.StorageAccount, storageKey)
}

func (d *Driver) newBlobContainerClient(ctx context.Context) (*container.Client, error) {
	keyCred, err := d.newSharedKeyCredential(ctx)
	if err != nil {
		return nil, err
	}
	containerURL := fmt.Sprintf("https://%s.blob.core.windows.net/%s", d.StorageAccount, d.BlobContainerName)
	return container.NewClientWithSharedKeyCredential(containerURL, keyCred, nil)
}

// IterateBlobs returns an iterator over blobs with the given prefix.
func (d *Driver) IterateBlobs(ctx context.Context, prefix string) (iter.Seq2[BlobItem, error], error) {
	client, err := d.newBlobContainerClient(ctx)
	if err != nil {
		return nil, err
	}
	pager := client.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: &prefix,
	})
	return func(yield func(BlobItem, error) bool) {
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				yield(BlobItem{}, err)
				return
			}
			for _, item := range page.Segment.BlobItems {
				if item.Name == nil {
					continue
				}
				var size int64
				if item.Properties != nil && item.Properties.ContentLength != nil {
					size = *item.Properties.ContentLength
				}
				if !yield(BlobItem{name: *item.Name, size: size}, nil) {
					return
				}
			}
		}
	}, nil
}

// DownloadBlob fetches the contents of a blob by name.
func (d *Driver) DownloadBlob(ctx context.Context, blobName string) ([]byte, error) {
	client, err := d.newBlobContainerClient(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.NewBlobClient(blobName).DownloadStream(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxBlobDownloadSize))
}
