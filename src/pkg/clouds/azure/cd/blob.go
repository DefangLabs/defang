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
	// Lazy init of storageKey can be racy when callers (e.g. CdList's worker
	// pool) call this concurrently. Hold the mutex across the ListKeys call so
	// only one goroutine fetches the key.
	d.storageKeyMu.Lock()
	defer d.storageKeyMu.Unlock()
	if d.storageKey == "" {
		d.storageKey = os.Getenv("AZURE_STORAGE_KEY")
	}
	if d.storageKey == "" {
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
		d.storageKey = *keys.Keys[0].Value
	}
	return azblob.NewSharedKeyCredential(d.StorageAccount, d.storageKey)
}

func (d *Driver) newBlobContainerClient(ctx context.Context, containerName string) (*container.Client, error) {
	keyCred, err := d.newSharedKeyCredential(ctx)
	if err != nil {
		return nil, err
	}
	containerURL := fmt.Sprintf("https://%s.blob.core.windows.net/%s", d.StorageAccount, containerName)
	return container.NewClientWithSharedKeyCredential(containerURL, keyCred, nil)
}

func (d *Driver) IterateBlobs(ctx context.Context, prefix string) (iter.Seq2[BlobItem, error], error) {
	return d.iterateBlobsInContainer(ctx, d.BlobContainerName, prefix)
}

// iterateBlobsInContainer is the container-explicit variant of IterateBlobs.
func (d *Driver) iterateBlobsInContainer(ctx context.Context, containerName, prefix string) (iter.Seq2[BlobItem, error], error) {
	client, err := d.newBlobContainerClient(ctx, containerName)
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

func (d *Driver) DownloadBlob(ctx context.Context, blobName string) ([]byte, error) {
	return d.downloadBlobFromContainer(ctx, d.BlobContainerName, blobName)
}

// downloadBlobFromContainer is the container-explicit variant of DownloadBlob.
func (d *Driver) downloadBlobFromContainer(ctx context.Context, containerName, blobName string) ([]byte, error) {
	client, err := d.newBlobContainerClient(ctx, containerName)
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
