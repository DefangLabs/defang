package cd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/google/uuid"
)

const prefix = "uploads/"

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
	blobName = prefix + blobName

	if _, err := d.SetUpStorageAccount(ctx); err != nil {
		return "", err
	}

	expiry := time.Now().UTC().Add(1 * time.Hour)

	keyCred, err := d.newSharedKeyCredential(ctx)
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
