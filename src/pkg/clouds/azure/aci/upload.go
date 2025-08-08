package aci

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

func (c *ContainerInstance) CreateUploadURL(ctx context.Context, blobName string) (string, error) {
	if blobName == "" {
		blobName = uuid.NewString()
	} else {
		if len(blobName) > 64 {
			return "", errors.New("name must be less than 64 characters")
		}
		// Sanitize the digest so it's safe to use as a file name
		blobName = strings.ReplaceAll(blobName, "/", "_")
		// name = path.Join(buildsPath, tenantName.String(), digest); TODO: avoid collisions between tenants
	}
	if _, err := c.SetUpStorageAccount(ctx); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	expiry := now.Add(1 * time.Hour)

	// TODO: using user delegation is more secure than shared key, but requires the user to reauth to a OAuth2 client with the appropriate permissions to discover the user's ObjectID
	// userCred, err := client.GetUserDelegationCredential(ctx, service.KeyInfo{
	// 	Start:  to.Ptr(now.Format(time.RFC3339)),
	// 	Expiry: to.Ptr(expiry.Format(time.RFC3339)),
	// }, nil)
	// if err != nil {
	// 	return "", err
	// }

	storageKey := os.Getenv("AZURE_STORAGE_KEY")
	if storageKey == "" {
		accountsClient, err := c.NewStorageAccountsClient()
		if err != nil {
			return "", err
		}

		keys, err := accountsClient.ListKeys(ctx, c.resourceGroupName, c.StorageAccount, nil)
		if err != nil {
			return "", err
		}
		storageKey = *keys.Keys[0].Value // or [1]?
	}

	keyCred, err := azblob.NewSharedKeyCredential(c.StorageAccount, storageKey)
	if err != nil {
		return "", err
	}

	// Create SAS; TODO: how does AZURE_STORAGE_SAS_TOKEN env var work?
	perms := sas.BlobPermissions{Create: true, Write: true, Read: true} // read is for ACR
	sasQueryParams, err := sas.BlobSignatureValues{
		BlobName:      blobName,
		ContainerName: c.BlobContainerName,
		ExpiryTime:    expiry,
		Permissions:   perms.String(),
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     now,
	}.SignWithSharedKey(keyCred)
	if err != nil {
		return "", err
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", c.StorageAccount)
	sasURL := fmt.Sprintf("%s%s/%s?%s", serviceURL, c.BlobContainerName, url.PathEscape(blobName), sasQueryParams.Encode())
	return sasURL, nil
}
