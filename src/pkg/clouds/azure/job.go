package azure

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/clouds"
)

type ContainerJob struct {
}

// var _ types.Driver = (*ContainerJob)(nil)

// CreateUploadURL implements types.Driver.
// func (c *ContainerJob) CreateUploadURL(ctx context.Context, name string) (string, error) {
// 	blobServiceClient, err := azblob.NewClient("", cred, nil)
// 	if err != nil {
// 		return "", err
// 	}

// 	containerClient := blobServiceClient.NewContainerClient(c.Location)
// 	blobClient := containerClient.NewBlobClient(name)

// 	url, err := blobClient.GetSASURL(ctx, azblob.BlobSASOptions{
// 		Permissions: azblob.BlobSASPermissions{Read: true, Write: true},
// 		Expiry:      time.Now().Add(1 * time.Hour),
// 	})
// 	if err != nil {
// 		return "", err
// 	}

// 	return url, nil

// }

// GetInfo implements clouds.Driver.
func (c *ContainerJob) GetInfo(ctx context.Context, taskID clouds.TaskID) (*clouds.TaskInfo, error) {
	panic("unimplemented")
}

// ListSecrets implements clouds.Driver.
func (c *ContainerJob) ListSecrets(ctx context.Context) ([]string, error) {
	panic("unimplemented")
}

// PutSecret implements clouds.Driver.
func (c *ContainerJob) PutSecret(ctx context.Context, name string, value string) error {
	panic("unimplemented")
}
