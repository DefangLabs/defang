package cd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/google/uuid"
)

func (d *Driver) CreateUploadURL(ctx context.Context, blobName string) (string, error) {
	if blobName == "" {
		blobName = uuid.NewString()
	} else {
		if len(blobName) > 64 {
			return "", errors.New("name must be less than 64 characters")
		}
		// Sanitize the digest so it's safe to use as a file name
		blobName = strings.ReplaceAll(blobName, "/", "_")
	}
	if _, err := d.SetUpStorageAccount(ctx); err != nil {
		return "", err
	}

	expiry := time.Now().UTC().Add(1 * time.Hour)

	storageKey := os.Getenv("AZURE_STORAGE_KEY")
	if storageKey == "" {
		accountsClient, err := d.NewStorageAccountsClient()
		if err != nil {
			return "", err
		}
		keys, err := accountsClient.ListKeys(ctx, d.resourceGroupName, d.StorageAccount, nil)
		if err != nil {
			return "", err
		}
		if len(keys.Keys) == 0 || keys.Keys[0].Value == nil {
			return "", errors.New("no storage account keys returned")
		}
		storageKey = *keys.Keys[0].Value
	}

	keyCred, err := azblob.NewSharedKeyCredential(d.StorageAccount, storageKey)
	if err != nil {
		return "", err
	}

	perms := sas.BlobPermissions{Create: true, Write: true, Read: true}
	sasQueryParams, err := sas.BlobSignatureValues{
		BlobName:      blobName,
		ContainerName: d.BlobContainerName,
		ExpiryTime:    expiry,
		Permissions:   perms.String(),
		Protocol:      sas.ProtocolHTTPS,
	}.SignWithSharedKey(keyCred)
	if err != nil {
		return "", err
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", d.StorageAccount)
	sasURL := fmt.Sprintf("%s%s/%s?%s", serviceURL, d.BlobContainerName, url.PathEscape(blobName), sasQueryParams.Encode())
	return sasURL, nil
}
